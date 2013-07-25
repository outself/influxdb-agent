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
	Name       string
	Regex      *regexp.Regexp
	StartCmd   string
	StopCmd    string
	StatusCmd  string
	User       string
	LastStatus Status
}
