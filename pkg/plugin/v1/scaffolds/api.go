/*
Copyright 2019 The Kubernetes Authors.

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

package scaffolds

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xenolf/lego/log"
	"k8s.io/client-go/discovery"
	crconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/kubebuilder/pkg/model"
	"sigs.k8s.io/kubebuilder/pkg/model/config"
	"sigs.k8s.io/kubebuilder/pkg/model/resource"
	"sigs.k8s.io/kubebuilder/pkg/plugin/scaffold"

	"github.com/joelanford/helm-operator/pkg/internal/kubebuilder/machinery"
	"github.com/joelanford/helm-operator/pkg/plugin/internal/chartutil"
	"github.com/joelanford/helm-operator/pkg/plugin/v1/scaffolds/internal/templates"
	"github.com/joelanford/helm-operator/pkg/plugin/v1/scaffolds/internal/templates/crd"
)

var _ scaffold.Scaffolder = &apiScaffolder{}

// apiScaffolder contains configuration for generating scaffolding for Go type
// representing the API and controller that implements the behavior for the API.
type apiScaffolder struct {
	config *config.Config
	opts   chartutil.CreateOptions
}

// NewAPIScaffolder returns a new Scaffolder for API/controller creation operations
func NewAPIScaffolder(config *config.Config, opts chartutil.CreateOptions) scaffold.Scaffolder {
	return &apiScaffolder{
		config: config,
		opts:   opts,
	}
}

// Scaffold implements Scaffolder
func (s *apiScaffolder) Scaffold() error {
	switch {
	case s.config.IsV3():
		return s.scaffold()
	default:
		return fmt.Errorf("unknown project version %v", s.config.Version)
	}
}

func (s *apiScaffolder) newUniverse(r *resource.Resource) *model.Universe {
	return model.NewUniverse(
		model.WithConfig(s.config),
		model.WithResource(r),
	)
}

func (s *apiScaffolder) scaffold() error {
	projectDir, err := os.Getwd()
	if err != nil {
		return err
	}
	r, chrt, err := chartutil.CreateChart(projectDir, s.opts)
	if err != nil {
		return err
	}

	// Check that resource doesn't exist
	if s.config.HasResource(r.GVK()) {
		return errors.New("API resource already exists")
	}
	// Check that the provided group can be added to the project
	if !s.config.MultiGroup && len(s.config.Resources) != 0 && !s.config.HasGroup(r.Group) {
		return fmt.Errorf("multiple groups are not allowed by default, to enable multi-group visit %s",
			"kubebuilder.io/migration/multi-group.html")
	}

	res := r.NewResource(s.config, true)
	s.config.AddResource(res.GVK())

	chartPath := filepath.Join(chartutil.HelmChartsDir, chrt.Metadata.Name)
	if err := machinery.NewScaffold().Execute(
		s.newUniverse(res),
		&templates.CRDSample{ChartPath: chartPath, Chart: chrt},
		&templates.CRDEditorRole{},
		&templates.CRDViewerRole{},
		&templates.WatchesUpdater{ChartPath: chartPath},
		&crd.CRD{CRDVersion: s.opts.CRDVersion},
	); err != nil {
		return fmt.Errorf("error scaffolding APIs: %v", err)
	}

	if err := machinery.NewScaffold().Execute(
		s.newUniverse(res),
		&crd.Kustomization{},
	); err != nil {
		return fmt.Errorf("error scaffolding kustomization: %v", err)
	}

	// TODO(joelanford): encapsulate this in the role discovery/generation into the scaffold?
	roleScaffold := templates.DefaultRoleScaffold
	if k8sCfg, err := crconfig.GetConfig(); err != nil {
		log.Warnf("Using default RBAC rules: failed to get Kubernetes config: %s", err)
	} else if dc, err := discovery.NewDiscoveryClientForConfig(k8sCfg); err != nil {
		log.Warnf("Using default RBAC rules: failed to create Kubernetes discovery client: %s", err)
	} else {
		roleScaffold = templates.GenerateRoleScaffold(dc, chrt)
	}

	if err := machinery.NewScaffold().Execute(
		s.newUniverse(res),
		&roleScaffold,
	); err != nil {
		return fmt.Errorf("error scaffolding role: %v", err)
	}

	if err = templates.MergeRoleForResource(res, projectDir, roleScaffold); err != nil {
		return fmt.Errorf("failed to merge rules in the RBAC manifest for resource (%s/%s, %v): %v",
			r.Group, r.Version, r.Kind, err)
	}

	return nil
}