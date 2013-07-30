package utils

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"regexp"
	"time"
)

var (
	Hostname           string
	UdpHost            string
	HttpHost           string
	Environment        string
	AppKey             string
	ApiKey             string
	Sleep              time.Duration
	Proxy              string
	LogFile            string
	LogLevel           string
	TopNProcesses      int
	MonitoredProcesses []*Process
	Plugins            []*Plugin
)

type Global struct {
	UdpHost            string `yaml:"udp-host"`
	HttpHost           string `yaml:"http-host"`
	ApiKey             string `yaml:"api-key"`
	AppKey             string `yaml:"app-key"`
	Environment        string
	Sleep              string
	Proxy              string
	LogFile            string     `yaml:"log-file"`
	LogLevel           string     `yaml:"log-level"`
	TopNProcesses      int        `yaml:"top-n-processes"`
	MonitoredProcesses []*Process `yaml:"processes,flow"`
	Plugins            []*Plugin  `yaml:"enabled-plugins,flow"`
}

func InitConfig(path string) error {
	var err error
	Hostname, err = os.Hostname()
	if err != nil {
		fmt.Printf("Cannot determine hostname. Error: %s\n", err)
		os.Exit(1)
	}

	content, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	m := Global{}
	err = goyaml.Unmarshal(content, &m)
	if err != nil {
		return err
	}

	// setPluginDefaults()
	// setProcessesDefaults()

	Sleep, err = time.ParseDuration(m.Sleep)
	if err != nil {
		return err
	}

	UdpHost = m.UdpHost
	HttpHost = m.HttpHost
	Environment = m.Environment
	AppKey = m.AppKey
	ApiKey = m.ApiKey
	Proxy = m.Proxy
	LogFile = m.LogFile
	LogLevel = m.LogLevel
	TopNProcesses = m.TopNProcesses
	MonitoredProcesses = m.MonitoredProcesses
	Plugins = m.Plugins

	for _, process := range MonitoredProcesses {
		process.CompiledRegex, err = regexp.Compile(process.Regex)
		if err != nil {
			return err
		}

		if process.StatusCmd == "" {
			process.StatusCmd = "name"
		}

		if process.User == "" {
			process.User = "root"
		}
	}

	for _, plugin := range Plugins {
		if plugin.Name == "" {
			return fmt.Errorf("Plugin name cannot be empty")
		}

		if len(plugin.Instances) == 0 {
			plugin.Instances = make([]*Instance, 0)
			plugin.Instances = append(plugin.Instances, &Instance{"default", nil, nil})
		}

		plugin.Cmd = fmt.Sprintf("/data/errplane-agent/plugins/%s/status", plugin.Name)

		for _, instance := range plugin.Instances {
			if len(instance.Args) > 0 {
				joinedArgs := make([]string, 0, len(instance.Args))
				for name, value := range instance.Args {
					joinedArgs = append(joinedArgs, "--"+name)
					joinedArgs = append(joinedArgs, value)
					instance.ArgsList = joinedArgs
				}
			}
		}
	}

	return nil
}
