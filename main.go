/*
Copyright 2020 The Operator-SDK Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	zapl "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/kubebuilder/pkg/cli"

	"github.com/joelanford/helm-operator/pkg/annotation"
	"github.com/joelanford/helm-operator/pkg/manager"
	v1 "github.com/joelanford/helm-operator/pkg/plugin/v1"
	"github.com/joelanford/helm-operator/pkg/reconciler"
	"github.com/joelanford/helm-operator/pkg/watches"
	"github.com/joelanford/helm-operator/version"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	klog.InitFlags(flag.CommandLine)

	r := &runner{}
	runCmd := cobra.Command{
		Use:   "run",
		Short: "Run an operator based on a Helm chart",
		Run:   r.run,
	}
	if err := r.BindFlags(runCmd.Flags()); err != nil {
		setupLog.Error(err, "failed to bind flags for run subcommand")
		os.Exit(1)
	}

	c, err := cli.New(
		cli.WithPlugins(&v1.Plugin{}),
		cli.WithDefaultPlugins(&v1.Plugin{}),
		cli.WithCommandName("helm-operator"),
		cli.WithExtraCommands(&runCmd),
	)
	if err != nil {
		setupLog.Error(err, "failed to setup CLI")
		os.Exit(1)
	}
	if err := c.Run(); err != nil {
		ctrl.Log.Error(err, "failed to run operator")
		os.Exit(1)
	}
}

func printVersion() {
	setupLog.Info("version information",
		"go", runtime.Version(),
		"GOOS", runtime.GOOS,
		"GOARCH", runtime.GOARCH,
		"helm-operator", version.Version)
}

type runner struct {
	metricsAddr             string
	enableLeaderElection    bool
	leaderElectionID        string
	leaderElectionNamespace string

	watchesFile                    string
	defaultMaxConcurrentReconciles int
	defaultReconcilePeriod         time.Duration

	// Deprecated: use defaultMaxConcurrentReconciles
	defaultMaxWorkers int
}

func (r *runner) BindFlags(set *pflag.FlagSet) error {
	set.StringVar(&r.metricsAddr, "metrics-addr", "0.0.0.0:8383", "The address the metric endpoint binds to.")
	set.BoolVar(&r.enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	set.StringVar(&r.leaderElectionID, "leader-election-id", "",
		"Name of the configmap that is used for holding the leader lock.")
	set.StringVar(&r.leaderElectionNamespace, "leader-election-namespace", "",
		"Namespace in which to create the leader election configmap for holding the leader lock (required if running locally).")

	set.StringVar(&r.watchesFile, "watches-file", "./watches.yaml", "Path to watches.yaml file.")
	set.DurationVar(&r.defaultReconcilePeriod, "reconcile-period", 0, "Default reconcile period for controllers (use 0 to disable periodic reconciliation)")
	set.IntVar(&r.defaultMaxConcurrentReconciles, "max-concurrent-reconciles", 1, "Default maximum number of concurrent reconciles for controllers.")

	// Deprecated: --max-workers flag does not align well with the name of the option it configures on the controller
	//   (MaxConcurrentReconciles). Flag `--max-concurrent-reconciles` should be used instead.
	set.IntVar(&r.defaultMaxWorkers, "max-workers", 1, "Default maximum number of concurrent reconciles for controllers.")
	if err := set.MarkHidden("max-workers"); err != nil {
		return err
	}
	return nil
}

func (r *runner) run(cmd *cobra.Command, _ []string) {
	logLvl := zap.NewAtomicLevelAt(zap.InfoLevel)
	sttLvl := zap.NewAtomicLevelAt(zap.PanicLevel)
	ctrl.SetLogger(zapl.New(
		zapl.UseDevMode(false),
		zapl.Level(&logLvl),
		zapl.StacktraceLevel(&sttLvl),
	))

	r.handleDeprecations(cmd)

	printVersion()

	options := ctrl.Options{
		MetricsBindAddress:      "0.0.0.0:8383",
		LeaderElection:          r.enableLeaderElection,
		LeaderElectionID:        r.leaderElectionID,
		LeaderElectionNamespace: r.leaderElectionNamespace,
		NewClient:               manager.NewDelegatingClientFunc(),
	}
	manager.ConfigureWatchNamespaces(&options, setupLog)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ws, err := watches.Load(r.watchesFile)
	if err != nil {
		setupLog.Error(err, "unable to load watches.yaml", "path", r.watchesFile)
		os.Exit(1)
	}

	for _, w := range ws {
		reconcilePeriod := r.defaultReconcilePeriod
		if w.ReconcilePeriod != nil {
			reconcilePeriod = *w.ReconcilePeriod
		}

		maxConcurrentReconciles := r.defaultMaxConcurrentReconciles
		if w.MaxConcurrentReconciles != nil {
			maxConcurrentReconciles = *w.MaxConcurrentReconciles
		}

		r, err := reconciler.New(
			reconciler.WithChart(*w.Chart),
			reconciler.WithGroupVersionKind(w.GroupVersionKind),
			reconciler.WithOverrideValues(w.OverrideValues),
			reconciler.SkipDependentWatches(w.WatchDependentResources != nil && !*w.WatchDependentResources),
			reconciler.WithMaxConcurrentReconciles(maxConcurrentReconciles),
			reconciler.WithReconcilePeriod(reconcilePeriod),
			reconciler.WithInstallAnnotations(annotation.DefaultInstallAnnotations...),
			reconciler.WithUpgradeAnnotations(annotation.DefaultUpgradeAnnotations...),
			reconciler.WithUninstallAnnotations(annotation.DefaultUninstallAnnotations...),
		)
		if err != nil {
			setupLog.Error(err, "unable to create helm reconciler", "controller", "Helm")
			os.Exit(1)
		}

		if err := r.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Helm")
			os.Exit(1)
		}
		setupLog.Info("configured watch", "gvk", w.GroupVersionKind, "chartPath", w.ChartPath, "maxConcurrentReconciles", maxConcurrentReconciles, "reconcilePeriod", reconcilePeriod)
	}

	// TODO(joelanford): kube-state-metrics?

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func (r *runner) handleDeprecations(cmd *cobra.Command) {
	// Deprecated: --max-workers flag does not align well with the name of the option it configures on the controller
	//   (MaxConcurrentReconciles). Flag `--max-concurrent-reconciles` should be used instead.
	if cmd.Flag("max-workers").Changed {
		setupLog.Info("flag --max-workers has been deprecated, use --max-concurrent-reconciles instead")
		if cmd.Flag("max-concurrent-reconciles").Changed {
			setupLog.Info("ignoring --max-workers since --max-concurrent-reconciles is set")
		} else {
			r.defaultMaxConcurrentReconciles = r.defaultMaxWorkers
		}
	}

	// Deprecated: OPERATOR_NAME environment variable is an artifact of the legacy operator-sdk project scaffolding.
	//   Flag `--leader-election-id` should be used instead.
	if operatorName, found := os.LookupEnv("OPERATOR_NAME"); found {
		setupLog.Info("environment variable OPERATOR_NAME has been deprecated, use --leader-election-id instead.")
		if cmd.Flag("leader-election-id").Changed {
			setupLog.Info("ignoring OPERATOR_NAME environment variable since --leader-election-id is set")
		} else {
			r.leaderElectionID = operatorName
		}
	}
}
