package utils

type Instance struct {
	Name     string
	Args     map[string]string
	ArgsList []string
}

type PluginMetadata struct {
	Name            string
	Verion          string
	Output          string
	HasDependencies bool     `yaml:"needs-dependencies"`
	Path            string   `yaml:"-"`
	IsCustom        bool     `yaml:"-"`
	CalculateRates  []string `yaml:"calculate-rates"`
}

type Plugin struct {
	Cmd       string
	Name      string
	Instances []*Instance
	Metadata  PluginMetadata
}
