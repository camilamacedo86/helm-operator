package v1

import (
	"sigs.k8s.io/kubebuilder/pkg/plugin"
)

const (
	pluginName    = "helm.sdk.operator-framework.io"
	pluginVersion = "v1.0.0"
)

var supportedProjectVersions = []string{"2"}

var (
	_ plugin.Base                  = Plugin{}
	_ plugin.InitPluginGetter      = Plugin{}
	_ plugin.CreateAPIPluginGetter = Plugin{}
)

type Plugin struct {
	initPlugin
	apiPlugin
}

func (Plugin) Name() string                           { return pluginName }
func (Plugin) Version() string                        { return pluginVersion }
func (Plugin) SupportedProjectVersions() []string     { return supportedProjectVersions }
func (p Plugin) GetInitPlugin() plugin.Init           { return &p.initPlugin }
func (p Plugin) GetCreateAPIPlugin() plugin.CreateAPI { return &p.apiPlugin }
