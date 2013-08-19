package utils

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"time"
)

type Config struct {
	Hostname          string `yaml:"-"`
	UdpHost           string `yaml:"udp-host"`
	HttpHost          string `yaml:"http-host"`
	ApiKey            string `yaml:"api-key"`
	AppKey            string `yaml:"app-key"`
	Environment       string
	Sleep             time.Duration `yaml:"-"`
	RawSleep          string        `yaml:"sleep"`
	TopNSleep         time.Duration `yaml:"-"`
	RawTopNSleep      string        `yaml:"top-n-sleep"`
	MonitoredSleep    time.Duration `yaml:"-"`
	RawMonitoredSleep string        `yaml:"monitored-sleep"`
	Proxy             string
	LogFile           string `yaml:"log-file"`
	LogLevel          string `yaml:"log-level"`
	ConfigService     string `yaml:"config-service"`
	TopNProcesses     int    `yaml:"top-n-processes"`

	// aggregator configuration
	Percentiles      []float64     `yaml:"percentiles,flow"`
	RawFlushInterval string        `yaml:"flush-interval"`
	FlushInterval    time.Duration `yaml:"-"`
	UdpAddr          string        `yaml:"udp-addr"`
}

func (self *Config) Database() string {
	return self.AppKey + self.Environment
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

	AgentConfig.TopNSleep, err = time.ParseDuration(AgentConfig.RawTopNSleep)
	if err != nil {
		return err
	}

	AgentConfig.MonitoredSleep, err = time.ParseDuration(AgentConfig.RawMonitoredSleep)
	if err != nil {
		return err
	}
	// for _, process := range AgentConfig.MonitoredProcesses {
	// 	process.CompiledRegex, err = regexp.Compile(process.Regex)
	// 	if err != nil {
	// 		return err
	// 	}

	// 	if process.StatusCmd == "" {
	// 		process.StatusCmd = "name"
	// 	}

	// 	if process.User == "" {
	// 		process.User = "root"
	// 	}
	// }

	// for _, plugin := range AgentConfig.Plugins {
	// 	if plugin.Name == "" {
	// 		return fmt.Errorf("Plugin name cannot be empty")
	// 	}

	// 	if len(plugin.Instances) == 0 {
	// 		plugin.Instances = make([]*Instance, 0)
	// 		plugin.Instances = append(plugin.Instances, &Instance{"default", nil, nil})
	// 	}

	// 	plugin.Cmd = fmt.Sprintf("/data/errplane-agent/plugins/%s/status", plugin.Name)
	// 	infoFile, err := ioutil.ReadFile(fmt.Sprintf("/data/errplane-agent/plugins/%s/info.yml", plugin.Name))
	// 	if err != nil {
	// 		fmt.Fprintf(os.Stderr, "info.yml wasn't found for %s plugin", plugin.Name)
	// 	}

	// 	err = goyaml.Unmarshal(infoFile, &plugin.Metadata)
	// 	if err != nil {
	// 		return err
	// 	}

	// 	for _, instance := range plugin.Instances {
	// 		if len(instance.Args) > 0 {
	// 			joinedArgs := make([]string, 0, len(instance.Args))
	// 			for name, value := range instance.Args {
	// 				joinedArgs = append(joinedArgs, "--"+name)
	// 				joinedArgs = append(joinedArgs, value)
	// 				instance.ArgsList = joinedArgs
	// 			}
	// 		}
	// 	}
	// }

	// return nil
	return nil
}
