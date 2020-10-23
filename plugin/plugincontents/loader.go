package plugincontents

import (
	"path/filepath"

	"fmt"

	"github.com/kube-aws/kube-aws/logger"
	"github.com/kube-aws/kube-aws/pkg/api"
	"github.com/kube-aws/kube-aws/provisioner"
)

type PluginFileLoader struct {
	p *api.Plugin

	FileLoader *provisioner.RemoteFileLoader
}

func NewPluginFileLoader(p *api.Plugin) *PluginFileLoader {
	return &PluginFileLoader{
		p: p,
	}
}

func (l *PluginFileLoader) String(f provisioner.RemoteFileSpec) (string, error) {
	if f.Source.Path != "" {
		f.Source.Path = filepath.Join("plugins", l.p.Name, f.Source.Path)
	}

	logger.Debugf("PluginFileLoader.String(): Calling load on FileLoader with RemoteFileSpec: %+v", f)
	loaded, err := l.FileLoader.Load(f)
	if err != nil {
		return "", err
	}

	res := loaded.Content.String()
	logger.Debugf("PluginFileLoader.String(): resultant string is: %+v", res)

	if f.Source.Path != "" && len(res) == 0 {
		return "", fmt.Errorf("[bug] empty file loaded from %s", f.Source.Path)
	}

	return res, nil
}
