package utils

import (
	"regexp"
)

type Status int

const (
	UP Status = iota
	DOWN
)

type Process struct {
	Name          string
	CompiledRegex *regexp.Regexp `yaml:"-"`
	Regex         string
	StartCmd      string `yaml:"start"`
	StopCmd       string `yaml:"stop"`
	StatusCmd     string `yaml:"status"`
	User          string
	LastStatus    Status
}
