package api

import (
	"github.com/kube-aws/kube-aws/logger"
	"github.com/kube-aws/kube-aws/plugin/pluginutil"
)

type PluginConfigs map[string]PluginConfig

func (pcs PluginConfigs) Merge(m PluginConfigs) (PluginConfigs, error) {
	var err error
	merged := PluginConfigs{}
	for name, pc := range pcs {
		merged[name] = pc
	}
	for name, pc := range m {
		logger.Debugf("PluginConfigs.Merge() Plugin %s: %+v", name, pc)
		merged[name], err = merged[name].Merge(pc)
		if err != nil {
			return merged, err
		}
	}
	return merged, nil
}

func (pcs PluginConfigs) PluginIsEnabled(name string) bool {
	var pc PluginConfig
	var ok bool
	if pc, ok = pcs[name]; !ok {
		return false
	}
	return pc.Enabled
}

func (pcs PluginConfigs) PluginExists(name string) bool {
	_, ok := pcs[name]
	return ok
}

type PluginConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
	Values  `yaml:",inline"`
}

func (p PluginConfig) Merge(m PluginConfig) (PluginConfig, error) {
	var err error
	result := p
	logger.Debugf("PluginConfig.Merge() %+v into %+v", m, p)
	result.Enabled = m.Enabled
	result.Values, err = pluginutil.MergeValues(p.Values, m.Values)
	logger.Debugf("PluginConfig.Merge() result %+v", result)
	return result, err
}
