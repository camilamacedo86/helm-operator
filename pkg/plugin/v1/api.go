package v1

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/kubebuilder/pkg/model/config"
	"sigs.k8s.io/kubebuilder/pkg/model/resource"
	"sigs.k8s.io/kubebuilder/pkg/plugin"

	"github.com/joelanford/helm-operator/pkg/watches"
)

type apiPlugin struct {
	config *config.Config
	ctx    *plugin.Context

	resource *resource.Options

	chartRef     string
	chartRepo    string
	chartVersion string
}

var (
	_ plugin.CreateAPI = &apiPlugin{}
)

const watchesFilePath = "watches.yaml"

func (p *apiPlugin) UpdateContext(ctx *plugin.Context) {
	ctx.Description = `Add an API to a helm project.
Updates the following files:
- watches.yaml with a new helm chart to API mapping.
Writes the following files:
- a new helm chart in the ./helm-charts/ directory.
- a Patch file for RBAC rules generated from the chart.

`
	ctx.Examples = fmt.Sprintf(`  $ %s create api \
      --group=my.domain
      --version=v1alpha1 \
      --kind=AppService

  $ %s create api \
      --version=v1alpha1 \
      --kind=AppService \
      --helm-chart=myrepo/app

  $ %s create api \
      --helm-chart=myrepo/app

  $ %s create api \
      --helm-chart=myrepo/app \
      --helm-chart-version=1.2.3

  $ %s create api \
      --helm-chart=app \
      --helm-chart-repo=https://charts.mycompany.com/

  $ %s create api \
      --helm-chart=app \
      --helm-chart-repo=https://charts.mycompany.com/ \
      --helm-chart-version=1.2.3

  $ %s create api \
      --helm-chart=/path/to/local/chart-directories/app/

  $ %s create api \
      --helm-chart=/path/to/local/chart-archives/app-1.2.3.tgz
`,
		ctx.CommandName, ctx.CommandName, ctx.CommandName, ctx.CommandName,
		ctx.CommandName, ctx.CommandName, ctx.CommandName, ctx.CommandName,
	)

	p.ctx = ctx
}

func (p *apiPlugin) BindFlags(fs *pflag.FlagSet) {
	p.resource = &resource.Options{}
	fs.StringVar(&p.resource.Kind, "kind", "", "resource Kind")
	fs.StringVar(&p.resource.Group, "group", "", "resource Group")
	fs.StringVar(&p.resource.Version, "version", "", "resource Version")

	fs.StringVar(&p.chartRef, "helm-chart", "", "chart reference (repo/name, directory, package file, or URL)")
	fs.StringVar(&p.chartRepo, "helm-chart-repo", "", "(optional) chart repository URL")
	fs.StringVar(&p.chartVersion, "helm-chart-version", "", "(optional) chart version (default: latest)")
}

func (p *apiPlugin) InjectConfig(c *config.Config) {
	p.config = c
}

func (p *apiPlugin) Run() error {
	if err := p.Validate(); err != nil {
		return err
	}

	if err := p.Scaffold(); err != nil {
		return err
	}

	return nil
}

func (p *apiPlugin) Validate() error {
	if len(p.chartRef) == 0 {
		if len(p.resource.Group) == 0 {
			return errors.New("--group is required unless --helm-chart is provided")
		}
		if len(p.resource.Version) == 0 {
			return errors.New("--version is required unless --helm-chart is provided")
		}
		if len(p.resource.Kind) == 0 {
			return errors.New("--kind is required unless --helm-chart is provided")
		}
		if len(p.chartRepo) != 0 {
			return errors.New("--helm-chart-repo only allowed with --helm-chart")
		}
		if len(p.chartVersion) != 0 {
			return errors.New("--helm-chart-version only allowed with --helm-chart")
		}
	}

	if err := p.resource.Validate(); err != nil {
		return fmt.Errorf("invalid resource: %w", err)
	}
	return nil
}

func (p *apiPlugin) Scaffold() error {
	ws, err := watches.Load(watchesFilePath)
	if err != nil {
		return err
	}

	gvk := schema.GroupVersionKind{
		Group:   fmt.Sprintf("%s.%s", p.resource.Group, p.config.Domain),
		Version: p.resource.Version,
		Kind:    p.resource.Kind,
	}

	for _, w := range ws {
		if w.GroupVersionKind == gvk {
			return fmt.Errorf("duplicate GVK: %s", w.GroupVersionKind)
		}
	}

	ws = append(ws, watches.Watch{
		GroupVersionKind: gvk,
		ChartPath:        filepath.Join("helm-charts", strings.ToLower(p.resource.Kind)),
	})

	data, err := yaml.Marshal(ws)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(watchesFilePath, data, 0644)
}
