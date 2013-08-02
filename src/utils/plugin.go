package utils

type Instance struct {
	Name     string
	Args     map[string]string
	ArgsList []string
}

type PluginMetadata struct {
	Verion          string
	Output          string
	HasDependencies bool `yaml:"needs-dependencies"`
}

type Plugin struct {
	Cmd       string
	Name      string
	Instances []*Instance
	Metadata  PluginMetadata
}
