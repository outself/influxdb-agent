package utils

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"regexp"
	"time"
)

type Config struct {
	Hostname           string `yaml:"-"`
	UdpHost            string `yaml:"udp-host"`
	HttpHost           string `yaml:"http-host"`
	ApiKey             string `yaml:"api-key"`
	AppKey             string `yaml:"app-key"`
	Environment        string
	Sleep              time.Duration `yaml:"-"`
	RawSleep           string        `yaml:"sleep"`
	Proxy              string
	LogFile            string     `yaml:"log-file"`
	LogLevel           string     `yaml:"log-level"`
	TopNProcesses      int        `yaml:"top-n-processes"`
	MonitoredProcesses []*Process `yaml:"processes,flow"`
	Plugins            []*Plugin  `yaml:"enabled-plugins,flow"`

	// aggregator configuration
	Percentiles      []float64     `yaml:"percentiles,flow"`
	RawFlushInterval string        `yaml:"flush-interval"`
	FlushInterval    time.Duration `yaml:"-"`
	UdpAddr          string        `yaml:"udp-addr"`
}

var AgentConfig Config

func InitConfig(path string) error {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	err = goyaml.Unmarshal(content, &AgentConfig)
	if err != nil {
		return err
	}

	AgentConfig.Hostname, err = os.Hostname()
	if err != nil {
		fmt.Printf("Cannot determine hostname. Error: %s\n", err)
		os.Exit(1)
	}

	// setPluginDefaults()
	// setProcessesDefaults()

	AgentConfig.Sleep, err = time.ParseDuration(AgentConfig.RawSleep)
	if err != nil {
		return err
	}

	AgentConfig.FlushInterval, err = time.ParseDuration(AgentConfig.RawFlushInterval)
	if err != nil {
		return err
	}

	for _, process := range AgentConfig.MonitoredProcesses {
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

	for _, plugin := range AgentConfig.Plugins {
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
