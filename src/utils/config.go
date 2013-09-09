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

	// datastore
	DatastoreDir string `yaml:"datastore-dir"`

	// websocket configuration
	RawWebsocketPing string `yaml:"websocket-ping"`
	WebsocketPing    time.Duration
	ConfigWebsocket  string `yaml:"config-websocket"`

	// aggregator configuration
	Percentiles      []float64     `yaml:"percentiles,flow"`
	RawFlushInterval string        `yaml:"flush-interval"`
	FlushInterval    time.Duration `yaml:"-"`
	UdpAddr          string        `yaml:"udp-addr"`
}

func (self *Config) Database() string {
	return self.AppKey + self.Environment
}

func ParseConfig(path string) (*Config, error) {
	config := &Config{}

	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = goyaml.Unmarshal(content, config)
	if err != nil {
		return nil, err
	}

	config.Hostname, err = os.Hostname()
	if err != nil {
		fmt.Printf("Cannot determine hostname. Error: %s\n", err)
		os.Exit(1)
	}

	// setPluginDefaults()
	// setProcessesDefaults()

	config.Sleep, err = time.ParseDuration(config.RawSleep)
	if err != nil {
		return nil, err
	}

	config.FlushInterval, err = time.ParseDuration(config.RawFlushInterval)
	if err != nil {
		return nil, err
	}

	config.TopNSleep, err = time.ParseDuration(config.RawTopNSleep)
	if err != nil {
		return nil, err
	}

	config.WebsocketPing, err = time.ParseDuration(config.RawWebsocketPing)
	if err != nil {
		return nil, err
	}

	config.MonitoredSleep, err = time.ParseDuration(config.RawMonitoredSleep)
	if err != nil {
		return nil, err
	}
	// for _, process := range config.MonitoredProcesses {
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

	// for _, plugin := range config.Plugins {
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
	return config, nil
}
