package utils

type Instance struct {
	Name string
	Args []string
}

type Plugin struct {
	Cmd       string
	Name      string
	Instances []*Instance
}
