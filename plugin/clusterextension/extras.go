package clusterextension

import (
	"fmt"

	"encoding/json"

	"github.com/kube-aws/kube-aws/pkg/api"
	"github.com/kube-aws/kube-aws/plugin/plugincontents"
	"github.com/kube-aws/kube-aws/plugin/pluginutil"
	"github.com/kube-aws/kube-aws/provisioner"
	"github.com/kube-aws/kube-aws/tmpl"

	//"os"
	"path/filepath"

	"github.com/kube-aws/kube-aws/logger"
)

type ClusterExtension struct {
	plugins []*api.Plugin
	Configs api.PluginConfigs
}

func NewExtrasFromPlugins(plugins []*api.Plugin, configs api.PluginConfigs) ClusterExtension {
	return ClusterExtension{
		plugins: plugins,
		Configs: configs,
	}
}

func NewExtras() ClusterExtension {
	return ClusterExtension{
		plugins: []*api.Plugin{},
		Configs: api.PluginConfigs{},
	}
}

type stack struct {
	Resources map[string]interface{}
	Outputs   map[string]interface{}
	Tags      map[string]interface{}
}

// KeyPairSpecs loads keypairs from enabled plugins with templating allowed in the dnsnames fields.
func (e ClusterExtension) KeyPairSpecs(renderContext interface{}) []api.KeyPairSpec {
	keypairs := []api.KeyPairSpec{}
	err := e.foreachEnabledPlugins(func(p *api.Plugin, pc *api.PluginConfig) error {
		values, err := pluginutil.MergeValues(p.Spec.Cluster.Values, pc.Values)
		if err != nil {
			return err
		}
		values, err = plugincontents.RenderTemplatesInValues(p.Metadata.Name, values, renderContext)
		if err != nil {
			return err
		}

		for _, spec := range p.Spec.Cluster.PKI.KeyPairs {
			var isTemplate bool
			var templatedDNSNames []string

			if isTemplate, err = plugincontents.LooksLikeATemplate(spec.CommonName); err != nil {
				return fmt.Errorf("failed to check common name '%s' for a template: %v", spec.CommonName, err)
			}
			if isTemplate {
				var tmpCN string
				if tmpCN, err = plugincontents.RenderStringFromTemplateWithValues(spec.CommonName, values, renderContext); err != nil {
					return fmt.Errorf("failed to render common name template '%s': %v", spec.CommonName, err)
				}
				spec.CommonName = tmpCN
			}

			if isTemplate, err = plugincontents.LooksLikeATemplate(spec.Organization); err != nil {
				return fmt.Errorf("failed to check organization '%s' for a template: %v", spec.Organization, err)
			}
			if isTemplate {
				var tmpO string
				if tmpO, err = plugincontents.RenderStringFromTemplateWithValues(spec.Organization, values, renderContext); err != nil {
					return fmt.Errorf("failed to render organization template '%s': %v", spec.Organization, err)
				}
				spec.Organization = tmpO
			}

			for _, dnsName := range spec.DNSNames {
				if isTemplate, err = plugincontents.LooksLikeATemplate(dnsName); err != nil {
					return fmt.Errorf("failed to check dnsname '%s' for a template: %v", dnsName, err)
				}
				if isTemplate {
					var renderedDNSName string
					if renderedDNSName, err = plugincontents.RenderStringFromTemplateWithValues(dnsName, values, renderContext); err != nil {
						return fmt.Errorf("failed to render dnsname template '%s': %v", dnsName, err)
					}
					dnsName = renderedDNSName
				}
				templatedDNSNames = append(templatedDNSNames, dnsName)
			}
			spec.DNSNames = templatedDNSNames
			keypairs = append(keypairs, spec)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return keypairs
}

func (e ClusterExtension) RootStack(renderContext, valuesContext interface{}) (*stack, error) {
	return e.stackExt("root", renderContext, valuesContext, func(p *api.Plugin) api.Stack {
		return p.Spec.Cluster.CloudFormation.Stacks.Root
	})
}

func (e ClusterExtension) NetworkStack(renderContext, valuesContext interface{}) (*stack, error) {
	return e.stackExt("network", renderContext, valuesContext, func(p *api.Plugin) api.Stack {
		return p.Spec.Cluster.CloudFormation.Stacks.Network
	})
}

type worker struct {
	ArchivedFiles       []provisioner.RemoteFileSpec
	CfnInitConfigSets   map[string]interface{}
	Files               []api.CustomFile
	SystemdUnits        []api.CustomSystemdUnit
	IAMPolicyStatements []api.IAMPolicyStatement
	NodeLabels          api.NodeLabels
	FeatureGates        api.FeatureGates
	Kubeconfig          string
	KubeletFlags        api.CommandLineFlags
	KubeletVolumeMounts []api.ContainerVolumeMount
}

type controller struct {
	ArchivedFiles       []provisioner.RemoteFileSpec
	APIServerFlags      api.CommandLineFlags
	APIServerVolumes    api.APIServerVolumes
	ControllerFlags     api.CommandLineFlags
	KubeProxyConfig     map[string]interface{}
	KubeSchedulerFlags  api.CommandLineFlags
	KubeletFlags        api.CommandLineFlags
	CfnInitConfigSets   map[string]interface{}
	Files               []api.CustomFile
	SystemdUnits        []api.CustomSystemdUnit
	IAMPolicyStatements []api.IAMPolicyStatement
	NodeLabels          api.NodeLabels
	Kubeconfig          string
	KubeletVolumeMounts []api.ContainerVolumeMount

	KubernetesManifestFiles []*provisioner.RemoteFile
	HelmReleaseFilesets     []api.HelmReleaseFileset
}

type etcd struct {
	Files               []api.CustomFile
	SystemdUnits        []api.CustomSystemdUnit
	IAMPolicyStatements []api.IAMPolicyStatement
}

func (e ClusterExtension) foreachEnabledPlugins(do func(p *api.Plugin, pc *api.PluginConfig) error) error {
	for _, p := range e.plugins {
		if enabled, pc := p.EnabledIn(e.Configs); enabled {
			if err := do(p, pc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e ClusterExtension) stackExt(name string, renderContext, valuesContext interface{}, src func(p *api.Plugin) api.Stack) (*stack, error) {
	resources := map[string]interface{}{}
	outputs := map[string]interface{}{}
	tags := map[string]interface{}{}

	err := e.foreachEnabledPlugins(func(p *api.Plugin, pc *api.PluginConfig) error {
		values, err := pluginutil.MergeValues(p.Spec.Cluster.Values, pc.Values)
		if err != nil {
			return err
		}
		values, err = plugincontents.RenderTemplatesInValues(p.Metadata.Name, values, valuesContext)
		if err != nil {
			return err
		}
		logger.Debugf("extras.go stackExt() resultant values: %+v", values)

		render := plugincontents.NewTemplateRenderer(p, values, renderContext)

		m, err := render.MapFromJsonContents(src(p).Resources.RemoteFileSpec)
		if err != nil {
			return fmt.Errorf("failed to load additional resources for %s stack: %v", name, err)
		}
		if l := len(m); l > 0 {
			logger.Infof("plugin %s extended stack %s with %d resources", p.Name, name, l)
		}
		for k, v := range m {
			resources[k] = v
		}

		m, err = render.MapFromJsonContents(src(p).Outputs.RemoteFileSpec)
		if err != nil {
			return fmt.Errorf("failed to load additional outputs for %s stack: %v", name, err)
		}
		if l := len(m); l > 0 {
			logger.Infof("plugin %s extended stack %s with %d outputs", p.Name, name, l)
		}
		for k, v := range m {
			outputs[k] = v
		}

		m, err = render.MapFromJsonContents(src(p).Tags.RemoteFileSpec)
		if err != nil {
			return fmt.Errorf("failed to load additional tags for %s stack: %v", name, err)
		}
		if l := len(m); l > 0 {
			logger.Infof("plugin %s extended stack %s with %d tags", p.Name, name, l)
		}
		for k, v := range m {
			tags[k] = v
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	logger.Debugf("PLUGINS: StackExt Additions for stack %s", name)
	logger.Debugf("Resources: %+v", resources)
	logger.Debugf("Outputs: %+v", outputs)
	logger.Debugf("Tags: %+v", tags)

	return &stack{
		Resources: resources,
		Outputs:   outputs,
		Tags:      tags,
	}, nil
}

func (e ClusterExtension) NodePoolStack(renderContext, valuesContext interface{}) (*stack, error) {
	logger.Debugf("Generating Plugin extras for nodepool cloudformation stack")
	return e.stackExt("node-pool", renderContext, valuesContext, func(p *api.Plugin) api.Stack {
		return p.Spec.Cluster.CloudFormation.Stacks.NodePool
	})
}

func renderMachineSystemdUnits(r *plugincontents.TemplateRenderer, systemd api.SystemdUnits) ([]api.CustomSystemdUnit, error) {
	result := []api.CustomSystemdUnit{}

	for _, d := range systemd {
		s, err := r.File(d.RemoteFileSpec)
		if err != nil {
			return nil, fmt.Errorf("failed to load systemd unit: %v", err)
		}

		u := api.CustomSystemdUnit{
			Name:    d.Name,
			Command: "start",
			Content: s,
			Enable:  true,
			Runtime: false,
		}
		result = append(result, u)
	}

	return result, nil
}

// simpleRenderMachineFiles - iterates over a plugin's MachineSpec.Files section loading/rendering contents to produce
// a list of custom files with their contents filled-in with the rendered results.
// NOTE1 - it does not use api.CustomFile's own template functionality (it is pre-rendered in the context of the plugin)
// NOTE2 - it knows nothing about binary files or configset files.
func simpleRenderMachineFiles(r *plugincontents.TemplateRenderer, files api.Files) ([]api.CustomFile, error) {
	result := []api.CustomFile{}

	for _, d := range files {
		s, err := r.File(d)
		if err != nil {
			return nil, fmt.Errorf("failed to load plugin etcd file contents: %v", err)
		}
		perm := d.Permissions
		f := api.CustomFile{
			Path:        d.Path,
			Permissions: perm,
			Content:     s,
		}
		result = append(result, f)
	}

	return result, nil
}

// renderMachineFilesAndConfigSets - the more complex partner of simpleRenderMachineFiles which also caters for binary files and configset files
func renderMachineFilesAndConfigSets(r *plugincontents.TemplateRenderer, files api.Files) ([]provisioner.RemoteFileSpec, []api.CustomFile, map[string]interface{}, error) {
	archivedFiles := []provisioner.RemoteFileSpec{}
	regularFiles := []api.CustomFile{}
	configsetFiles := make(map[string]interface{})

	for _, d := range files {
		if d.IsBinary() {
			archivedFiles = append(archivedFiles, d)
			continue
		}

		s, cfg, err := regularOrConfigSetFile(d, r)
		if err != nil {
			return archivedFiles, regularFiles, configsetFiles, fmt.Errorf("failed to load plugin worker file contents: %v", err)
		}

		var perm uint
		perm = d.Permissions

		if s != nil {
			f := api.CustomFile{
				Path:        d.Path,
				Permissions: perm,
				Content:     *s,
				Type:        d.Type,
			}
			regularFiles = append(regularFiles, f)
		} else {
			configsetFiles[d.Path] = map[string]interface{}{
				"content": cfg,
			}
		}
	}
	return archivedFiles, regularFiles, configsetFiles, nil
}

func regularOrConfigSetFile(f provisioner.RemoteFileSpec, render *plugincontents.TemplateRenderer) (*string, map[string]interface{}, error) {
	logger.Debugf("regularOrConfigSetFile(): Reading remoteFileSpec: %+v", f)
	goRendered, err := render.File(f)
	if err != nil {
		return nil, nil, err
	}

	// Disable templates for files that are known to be static e.g. binaries and credentials
	// Type "binary" is not considered here because binaries are too huge to be handled here.
	if f.Type == "credential" {
		return &goRendered, nil, nil
	}

	tokens := tmpl.TextToCfnExprTokens(goRendered)

	logger.Debugf("Number of tokens produced: %d", len(tokens))
	if len(tokens) == 0 {
		return &goRendered, nil, nil
	}
	if len(tokens) == 1 {
		var out string
		if err := json.Unmarshal([]byte(tokens[0]), &out); err != nil {
			logger.Errorf("failed unmarshalling %s from json: %v", tokens[0], err)
			return nil, nil, err
		}
		return &out, nil, nil
	}

	return nil, map[string]interface{}{"Fn::Join": []interface{}{"", tokens}}, nil
}

func (e ClusterExtension) Worker(renderContext interface{}) (*worker, error) {
	logger.Debugf("ClusterExtension.Worker(): Generating Plugin Worker user-data extras")
	files := []api.CustomFile{}
	systemdUnits := []api.CustomSystemdUnit{}
	iamStatements := []api.IAMPolicyStatement{}
	nodeLabels := api.NodeLabels{}
	featureGates := api.FeatureGates{}
	configsets := map[string]interface{}{}
	archivedFiles := []provisioner.RemoteFileSpec{}
	var kubeconfig string
	kubeletFlags := api.CommandLineFlags{}
	kubeletMounts := []api.ContainerVolumeMount{}

	for _, p := range e.plugins {
		if enabled, pc := p.EnabledIn(e.Configs); enabled {
			logger.Debugf("Adding worker extensions from plugin %s", p.Name)
			values, err := pluginutil.MergeValues(p.Spec.Cluster.Values, pc.Values)
			if err != nil {
				return nil, err
			}
			values, err = plugincontents.RenderTemplatesInValues(p.Metadata.Name, values, renderContext)
			if err != nil {
				return nil, err
			}
			render := plugincontents.NewTemplateRenderer(p, values, renderContext)

			extraUnits, err := renderMachineSystemdUnits(render, p.Spec.Cluster.Machine.Roles.Worker.Systemd.Units)
			if err != nil {
				return nil, fmt.Errorf("failed adding systemd units to worker: %v", err)
			}
			if l := len(extraUnits); l > 0 {
				logger.Infof("plugin %s added %d extra worker systemd units", p.Name, l)
			}
			systemdUnits = append(systemdUnits, extraUnits...)

			extraArchivedFiles, extraFiles, extraConfigSetFiles, err := renderMachineFilesAndConfigSets(render, p.Spec.Cluster.Roles.Worker.Files)
			if err != nil {
				return nil, fmt.Errorf("failed adding files to worker: %v", err)
			}
			if l := len(extraArchivedFiles); l > 0 {
				logger.Infof("plugin %s added %d extra worker extra archive files", p.Name, l)
			}
			if l := len(extraFiles); l > 0 {
				logger.Infof("plugin %s added %d extra worker extra files", p.Name, l)
			}
			if l := len(extraConfigSetFiles); l > 0 {
				logger.Infof("plugin %s added %d extra worker extra config-set files", p.Name, l)
			}
			archivedFiles = append(archivedFiles, extraArchivedFiles...)
			files = append(files, extraFiles...)
			configsets[p.Name] = map[string]map[string]interface{}{
				"files": extraConfigSetFiles,
			}

			if l := len(p.Spec.Cluster.Machine.Roles.Worker.IAM.Policy.Statements); l > 0 {
				logger.Infof("plugin %s added %d extra worker iam policies", p.Name, l)
			}
			iamStatements = append(iamStatements, p.Spec.Cluster.Machine.Roles.Worker.IAM.Policy.Statements...)

			if l := len(p.Spec.Cluster.Machine.Roles.Worker.Kubelet.NodeLabels); l > 0 {
				logger.Infof("plugin %s added %d extra worker node labels", p.Name, l)
			}
			for k, v := range p.Spec.Cluster.Machine.Roles.Worker.Kubelet.NodeLabels {
				nodeLabels[k] = v
			}

			if l := len(p.Spec.Cluster.Machine.Roles.Worker.Kubelet.FeatureGates); l > 0 {
				logger.Infof("plugin %s added %d extra worker kubelet feature gates", p.Name, l)
			}
			for k, v := range p.Spec.Cluster.Machine.Roles.Worker.Kubelet.FeatureGates {
				featureGates[k] = v
			}

			if p.Spec.Cluster.Machine.Roles.Worker.Kubelet.Kubeconfig != "" {
				logger.Infof("plugin %s changed the worker kubeconfig", p.Name)
				kubeconfig = p.Spec.Cluster.Machine.Roles.Worker.Kubelet.Kubeconfig
			}

			if len(p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Mounts) > 0 {
				logger.Infof("plugin %s added %d worker kubelet mounts", p.Name, len(p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Mounts))
				kubeletMounts = append(kubeletMounts, p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Mounts...)
			}

			extraKubeletFlags, err := getFlags(render, p.Spec.Cluster.Kubernetes.Kubelet.Flags)
			if l := len(extraKubeletFlags); l > 0 {
				logger.Infof("plugin %s added %d extra worker kubelet command-line flags", p.Name, l)
			}
			if err != nil {
				return nil, err
			}
			kubeletFlags = append(kubeletFlags, extraKubeletFlags...)
		}
	}

	return &worker{
		ArchivedFiles:       archivedFiles,
		CfnInitConfigSets:   configsets,
		Files:               files,
		SystemdUnits:        systemdUnits,
		IAMPolicyStatements: iamStatements,
		NodeLabels:          nodeLabels,
		FeatureGates:        featureGates,
		KubeletVolumeMounts: kubeletMounts,
		KubeletFlags:        kubeletFlags,
		Kubeconfig:          kubeconfig,
	}, nil
}

func (e ClusterExtension) ControlPlaneStack(renderContext, valuesContext interface{}) (*stack, error) {
	return e.stackExt("control-plane", renderContext, valuesContext, func(p *api.Plugin) api.Stack {
		return p.Spec.Cluster.CloudFormation.Stacks.ControlPlane
	})
}

func (e ClusterExtension) EtcdStack(renderContext, valuesContext interface{}) (*stack, error) {
	return e.stackExt("etcd", renderContext, valuesContext, func(p *api.Plugin) api.Stack {
		return p.Spec.Cluster.CloudFormation.Stacks.Etcd
	})
}

// renderKubernetesManifests - yet another specialised function for rendering provisioner.RemoteFileSpec this time into kubernetes manifests
func renderKubernetesManifests(pluginName string, r *plugincontents.TemplateRenderer, mspecs api.KubernetesManifests) ([]api.CustomFile, []*provisioner.RemoteFile, map[string]interface{}, error) {
	files := []api.CustomFile{}
	manifests := []*provisioner.RemoteFile{}
	configsetFiles := make(map[string]interface{})

	for _, m := range mspecs {
		rendered, ma, err := regularOrConfigSetFile(m.RemoteFileSpec, r)
		if err != nil {
			return files, manifests, configsetFiles, fmt.Errorf("Failed to render plugin kubernetes manifest: %v", err)
		}
		var name string
		if m.Name == "" {
			if m.RemoteFileSpec.Source.Path == "" {
				return files, manifests, configsetFiles, fmt.Errorf("manifest.name is required in %v", m)
			}
			name = filepath.Base(m.RemoteFileSpec.Source.Path)
		} else {
			name = m.Name
		}

		remotePath := filepath.Join("/srv/kube-aws/plugins", pluginName, name)
		if rendered == nil {
			configsetFiles[remotePath] = map[string]interface{}{
				"content": ma,
			}
			manifests = append(manifests, provisioner.NewRemoteFileAtPath(remotePath, []byte{}))
			continue
		}

		f := api.CustomFile{
			Path:        remotePath,
			Permissions: 0644,
			Content:     *rendered,
		}
		files = append(files, f)
		manifests = append(manifests, provisioner.NewRemoteFileAtPath(f.Path, []byte(f.Content)))
	}

	return files, manifests, configsetFiles, nil
}

func renderHelmReleases(pluginName string, releases api.HelmReleases) ([]api.HelmReleaseFileset, error) {
	releaseFileSets := []api.HelmReleaseFileset{}

	for _, releaseConfig := range releases {
		valuesFilePath := filepath.Join("/srv/kube-aws/plugins", pluginName, "helm", "releases", releaseConfig.Name, "values.yaml")
		valuesFileContent, err := json.Marshal(releaseConfig.Values)
		if err != nil {
			return releaseFileSets, fmt.Errorf("Unexpected error in HelmReleasePlugin: %v", err)
		}
		releaseFileData := map[string]interface{}{
			"values": map[string]string{
				"file": valuesFilePath,
			},
			"chart": map[string]string{
				"name":    releaseConfig.Name,
				"version": releaseConfig.Version,
			},
		}
		releaseFilePath := filepath.Join("/srv/kube-aws/plugins", pluginName, "helm", "releases", releaseConfig.Name, "release.json")
		releaseFileContent, err := json.Marshal(releaseFileData)
		if err != nil {
			return releaseFileSets, fmt.Errorf("Unexpected error in HelmReleasePlugin: %v", err)
		}
		r := api.HelmReleaseFileset{
			ValuesFile: provisioner.NewRemoteFileAtPath(
				valuesFilePath,
				valuesFileContent,
			),
			ReleaseFile: provisioner.NewRemoteFileAtPath(
				releaseFilePath,
				releaseFileContent,
			),
		}
		releaseFileSets = append(releaseFileSets, r)
	}
	return releaseFileSets, nil
}

// getFlags - generic loader of command line flags, returns a slice of flags
func getFlags(render *plugincontents.TemplateRenderer, flags api.CommandLineFlags) ([]api.CommandLineFlag, error) {
	extraFlags := []api.CommandLineFlag{}

	for _, f := range flags {
		v, err := render.String(f.Value)
		if err != nil {
			return extraFlags, fmt.Errorf("failed to load apisersver flags: %v", err)
		}
		newFlag := api.CommandLineFlag{
			Name:  f.Name,
			Value: v,
		}
		extraFlags = append(extraFlags, newFlag)
	}
	return extraFlags, nil
}

func (e ClusterExtension) Controller(renderContext interface{}) (*controller, error) {
	logger.Debugf("ClusterExtension.Controller(): Generating Plugin Controller user-data extras")
	apiServerFlags := api.CommandLineFlags{}
	apiServerVolumes := api.APIServerVolumes{}
	controllerFlags := api.CommandLineFlags{}
	kubeProxyConfig := map[string]interface{}{}
	kubeletFlags := api.CommandLineFlags{}
	kubeSchedulerFlags := api.CommandLineFlags{}

	systemdUnits := []api.CustomSystemdUnit{}
	files := []api.CustomFile{}
	iamStatements := api.IAMPolicyStatements{}
	nodeLabels := api.NodeLabels{}
	configsets := map[string]interface{}{}
	archivedFiles := []provisioner.RemoteFileSpec{}
	var kubeconfig string
	kubeletMounts := []api.ContainerVolumeMount{}
	manifests := []*provisioner.RemoteFile{}
	releaseFilesets := []api.HelmReleaseFileset{}

	for _, p := range e.plugins {
		//fmt.Fprintf(os.Stderr, "plugin=%+v configs=%+v", p, e.configs)
		if enabled, pc := p.EnabledIn(e.Configs); enabled {
			logger.Debugf("Adding controller extensions from plugin %s", p.Name)
			values, err := pluginutil.MergeValues(p.Spec.Cluster.Values, pc.Values)
			if err != nil {
				return nil, err
			}
			values, err = plugincontents.RenderTemplatesInValues(p.Metadata.Name, values, renderContext)
			if err != nil {
				return nil, err
			}
			render := plugincontents.NewTemplateRenderer(p, values, renderContext)

			extraApiServerFlags, err := getFlags(render, p.Spec.Cluster.Kubernetes.APIServer.Flags)
			if err != nil {
				return nil, err
			}
			if l := len(extraApiServerFlags); l > 0 {
				logger.Infof("plugin %s added %d extra controller api server command-line flags", p.Name, l)
			}
			apiServerFlags = append(apiServerFlags, extraApiServerFlags...)

			extraControllerManagerFlags, err := getFlags(render, p.Spec.Cluster.Kubernetes.ControllerManager.Flags)
			if err != nil {
				return nil, err
			}
			if l := len(extraControllerManagerFlags); l > 0 {
				logger.Infof("plugin %s added %d extra controller controller-manager command-line flags", p.Name, l)
			}
			controllerFlags = append(controllerFlags, extraControllerManagerFlags...)

			extraKubeSchedulerFlags, err := getFlags(render, p.Spec.Cluster.Kubernetes.KubeScheduler.Flags)
			if err != nil {
				return nil, err
			}
			if l := len(extraKubeSchedulerFlags); l > 0 {
				logger.Infof("plugin %s added %d extra controller scheduler command-line flags", p.Name, l)
			}
			kubeSchedulerFlags = append(kubeSchedulerFlags, extraKubeSchedulerFlags...)

			extraKubeletFlags, err := getFlags(render, p.Spec.Cluster.Kubernetes.Kubelet.Flags)
			if err != nil {
				return nil, err
			}
			if l := len(extraKubeSchedulerFlags); l > 0 {
				logger.Infof("plugin %s added %d extra controller kubelet command-line flags", p.Name, l)
			}
			kubeletFlags = append(kubeletFlags, extraKubeletFlags...)

			for key, value := range p.Spec.Cluster.Kubernetes.KubeProxy.Config {
				kubeProxyConfig[key] = value
			}
			if l := len(p.Spec.Cluster.Kubernetes.KubeProxy.Config); l > 0 {
				logger.Infof("plugin %s added %d extra controller kube-proxy configuration keys", p.Name, l)
			}

			apiServerVolumes = append(apiServerVolumes, p.Spec.Cluster.Kubernetes.APIServer.Volumes...)
			if l := len(p.Spec.Cluster.Kubernetes.APIServer.Volumes); l > 0 {
				logger.Infof("plugin %s added %d extra controller volumes", p.Name, l)
			}

			extraUnits, err := renderMachineSystemdUnits(render, p.Spec.Cluster.Machine.Roles.Controller.Systemd.Units)
			if err != nil {
				return nil, fmt.Errorf("failed adding systemd units to etcd: %v", err)
			}
			if l := len(extraUnits); l > 0 {
				logger.Infof("plugin %s added %d extra controller systemd units", p.Name, l)
			}
			systemdUnits = append(systemdUnits, extraUnits...)

			extraArchivedFiles, extraFiles, extraConfigSetFiles, err := renderMachineFilesAndConfigSets(render, p.Spec.Cluster.Roles.Controller.Files)
			if err != nil {
				return nil, fmt.Errorf("failed adding files to controller: %v", err)
			}
			if l := len(extraArchivedFiles); l > 0 {
				logger.Infof("plugin %s added %d extra controller extra archive files", p.Name, l)
			}
			if l := len(extraFiles); l > 0 {
				logger.Infof("plugin %s added %d extra controller extra files", p.Name, l)
			}
			if l := len(extraConfigSetFiles); l > 0 {
				logger.Infof("plugin %s added %d extra controller extra config-set files", p.Name, l)
			}
			archivedFiles = append(archivedFiles, extraArchivedFiles...)
			files = append(files, extraFiles...)

			if l := len(p.Spec.Cluster.Machine.Roles.Controller.IAM.Policy.Statements); l > 0 {
				logger.Infof("plugin %s added %d extra controller iam policies", p.Name, l)
			}
			iamStatements = append(iamStatements, p.Spec.Cluster.Machine.Roles.Controller.IAM.Policy.Statements...)

			if l := len(p.Spec.Cluster.Machine.Roles.Controller.Kubelet.NodeLabels); l > 0 {
				logger.Infof("plugin %s added %d extra controller node labels", p.Name, l)
			}
			for k, v := range p.Spec.Cluster.Machine.Roles.Controller.Kubelet.NodeLabels {
				nodeLabels[k] = v
			}

			if p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Kubeconfig != "" {
				logger.Infof("plugin %s changed the controller kubeconfig", p.Name)
				kubeconfig = p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Kubeconfig
			}

			if len(p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Mounts) > 0 {
				logger.Infof("plugin %s added %d controller kubelet mounts", p.Name, len(p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Mounts))
				kubeletMounts = append(kubeletMounts, p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Mounts...)
			}

			logger.Debugf("Rendering Controller files and manifests...")
			extraFiles, extraManifests, manifestConfigSetFiles, err := renderKubernetesManifests(p.Name, render, p.Spec.Cluster.Kubernetes.Manifests)
			if err != nil {
				return nil, fmt.Errorf("failed adding kubernetes manifests to controller: %v", err)
			}
			files = append(files, extraFiles...)
			manifests = append(manifests, extraManifests...)
			// merge the manifest configsets into machine generated configsetfiles
			for k, v := range manifestConfigSetFiles {
				extraConfigSetFiles[k] = v
			}
			if l := len(extraManifests); l > 0 {
				logger.Infof("plugin %s added %d extra kubernetes manifests", p.Name, l)
			}
			configsets[p.Name] = map[string]map[string]interface{}{
				"files": extraConfigSetFiles,
			}

			extraReleaseFileSets, err := renderHelmReleases(p.Name, p.Spec.Cluster.Helm.Releases)
			releaseFilesets = append(releaseFilesets, extraReleaseFileSets...)
		}
	}

	return &controller{
		ArchivedFiles:           archivedFiles,
		APIServerFlags:          apiServerFlags,
		ControllerFlags:         controllerFlags,
		KubeSchedulerFlags:      kubeSchedulerFlags,
		KubeProxyConfig:         kubeProxyConfig,
		KubeletFlags:            kubeletFlags,
		APIServerVolumes:        apiServerVolumes,
		Files:                   files,
		SystemdUnits:            systemdUnits,
		IAMPolicyStatements:     iamStatements,
		NodeLabels:              nodeLabels,
		KubeletVolumeMounts:     kubeletMounts,
		Kubeconfig:              kubeconfig,
		CfnInitConfigSets:       configsets,
		KubernetesManifestFiles: manifests,
		HelmReleaseFilesets:     releaseFilesets,
	}, nil
}

func (e ClusterExtension) Etcd(renderContext interface{}) (*etcd, error) {
	logger.Debugf("ClusterExtension.Etcd(): Generating Plugin Etcd user-data extras")
	systemdUnits := []api.CustomSystemdUnit{}
	files := []api.CustomFile{}
	iamStatements := api.IAMPolicyStatements{}

	for _, p := range e.plugins {
		if enabled, pc := p.EnabledIn(e.Configs); enabled {
			logger.Debugf("Adding etcd extensions from plugin %s", p.Name)
			values, err := pluginutil.MergeValues(p.Spec.Cluster.Values, pc.Values)
			if err != nil {
				return nil, err
			}
			values, err = plugincontents.RenderTemplatesInValues(p.Metadata.Name, values, renderContext)
			if err != nil {
				return nil, err
			}
			render := plugincontents.NewTemplateRenderer(p, values, renderContext)

			extraUnits, err := renderMachineSystemdUnits(render, p.Spec.Cluster.Machine.Roles.Etcd.Systemd.Units)
			if err != nil {
				return nil, fmt.Errorf("failed adding systemd units to etcd: %v", err)
			}
			if l := len(extraUnits); l > 0 {
				logger.Infof("plugin %s added %d extra etcd systemd units", p.Name, l)
			}
			systemdUnits = append(systemdUnits, extraUnits...)

			extraFiles, err := simpleRenderMachineFiles(render, p.Spec.Cluster.Roles.Etcd.Files)
			if err != nil {
				return nil, fmt.Errorf("failed adding files to etcd: %v", err)
			}
			if l := len(extraFiles); l > 0 {
				logger.Infof("plugin %s added %d extra etcd files", p.Name, l)
			}
			files = append(files, extraFiles...)

			iamStatements = append(iamStatements, p.Spec.Cluster.Roles.Etcd.IAM.Policy.Statements...)
		}
	}

	return &etcd{
		Files:               files,
		SystemdUnits:        systemdUnits,
		IAMPolicyStatements: iamStatements,
	}, nil
}
