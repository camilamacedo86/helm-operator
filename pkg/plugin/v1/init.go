package v1

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/kubebuilder/pkg/model/config"
	"sigs.k8s.io/kubebuilder/pkg/plugin"
)

type initPlugin struct {
	config *config.Config
	ctx    *plugin.Context

	// boilerplate options
	license string
	owner   string
}

var (
	_ plugin.Init = &initPlugin{}
)

func (p *initPlugin) UpdateContext(ctx *plugin.Context) {
	ctx.Description = `Initialize a new helm project.
Writes the following files:
- a PROJECT file with the domain and repo
- a Makefile to build the project
- a Kustomization.yaml for customizating manifests
- a Patch file for customizing image for manager manifests
- a Patch file for enabling prometheus metrics

`
	ctx.Examples = fmt.Sprintf(`  # Scaffold a project using the apache2 license with "The Kubernetes authors" as owners
  %s init --plugin=helm.sdk.operator-framework.io:v1 --domain example.org --license apache2 --owner "The Kubernetes authors"
`,
		ctx.CommandName)

	p.ctx = ctx
}

func (p *initPlugin) BindFlags(fs *pflag.FlagSet) {
	// project args
	fs.StringVar(&p.config.Domain, "domain", "my.domain", "domain for groups")
}

func (p *initPlugin) InjectConfig(c *config.Config) {
	p.config = c
	p.config.Layout = fmt.Sprintf("%s", pluginName)
}

func (p *initPlugin) Run() error {
	if err := p.Validate(); err != nil {
		return err
	}

	if err := p.Scaffold(); err != nil {
		return err
	}

	return p.PostScaffold()
}

func (p *initPlugin) Validate() error {
	// Check if the project name is a valid namespace according to k8s
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("error to get the current path: %v", err)
	}
	projectName := filepath.Base(dir)
	if err := validation.IsDNS1123Label(strings.ToLower(projectName)); err != nil {
		return fmt.Errorf("project name (%s) is invalid: %v", projectName, err)
	}

	if err := validation.IsDNS1123Subdomain(p.config.Domain); err != nil {
		return fmt.Errorf("domain (%s) is invalid: %v", p.config.Domain, err)
	}
	return nil
}

func (p *initPlugin) Scaffold() error {
	if err := os.Mkdir("helm-charts", 0755); err != nil {
		return err
	}

	watchesContent := fmt.Sprintf(`# This file contains the configured watches for the helm-based operator.
# To add new watches, use %s create api
`, p.ctx.CommandName)

	if err := ioutil.WriteFile("watches.yaml", []byte(watchesContent), 0644); err != nil {
		return err
	}
	return nil
}

func (p *initPlugin) PostScaffold() error {
	fmt.Printf("Next: define a resource with:\n$ %s create api\n", p.ctx.CommandName)
	return nil
}
