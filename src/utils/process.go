package utils

type Status int

const (
	UP Status = iota
	DOWN
)

type Process struct {
	Name       string
	StartCmd   string
	StopCmd    string
	StatusCmd  string
	User       string
	LastStatus Status
}
