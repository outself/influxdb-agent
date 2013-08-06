package utils

type Status int

const (
	UP Status = iota
	DOWN
)

type Process struct {
	Name       string `json:"name"`
	Regex      string `json:"regex"`
	StartCmd   string `json:"start"`
	StopCmd    string `json:"stop"`
	StatusCmd  string `json:"status"`
	User       string `json:"user"`
	LastStatus Status `json:"-"`
}
