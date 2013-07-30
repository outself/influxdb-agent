package utils

type Instance struct {
	Name     string
	Args     map[string]string
	ArgsList []string
}

type Plugin struct {
	Cmd       string
	Name      string
	Instances []*Instance
}
