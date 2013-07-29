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
	m := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(content, &m)
	if err != nil {
		return err
	}
	general := m["general"].(map[interface{}]interface{})
	UdpHost = general["udp-host"].(string)
	HttpHost = general["http-host"].(string)
	Environment = general["environment"].(string)
	AppKey = general["app-key"].(string)
	ApiKey = general["api-key"].(string)
	sleepStr := general["sleep"].(string)
	Sleep, err = time.ParseDuration(sleepStr)
	if err != nil {
		return err
	}
	proxy_ := general["proxy"]
	if proxy_ != nil {
		Proxy = proxy_.(string)
	}
	LogFile = general["log-file"].(string)
	LogLevel = general["log-level"].(string)
	TopNProcesses = general["top-n-processes"].(int)

	// FIXME: this should come from the backend

	// get the processes that we should monitor
	processes := m["processes"]

	if processes != nil {
		if err := parseProcesse(processes); err != nil {
			return err
		}
	}

	// FIXME: this should come from the backend

	plugins := m["plugins"]

	if plugins != nil {
		if err := parsePlugins(plugins); err != nil {
			return err
		}
	}

	return nil
}

func parsePlugins(plugins interface{}) error {
	for name, _plugin := range plugins.(map[interface{}]interface{}) {
		plugin := _plugin.(map[interface{}]interface{})
		if plugin["name"] != nil {
			name = plugin["name"].(string)
		}

		cmd := plugin["cmd"].(string)
		Plugins = append(Plugins, &Plugin{cmd, name.(string)})
	}

	return nil
}

func parseProcesse(processes interface{}) error {
	for _, process := range processes.([]interface{}) {
		var name, startCmd, stopCmd, statusCmd, user string
		var regex *regexp.Regexp
		var err error
		switch x := process.(type) {
		case map[interface{}]interface{}:
			if len(x) != 1 {
				return fmt.Errorf("Bad configuration file at %v", x)
			}
			for processName, _specs := range x {
				name = processName.(string)
				specs := _specs.(map[interface{}]interface{})
				if cmd, ok := specs["start"]; ok {
					startCmd = cmd.(string)
				}
				if cmd, ok := specs["stop"]; ok {
					stopCmd = cmd.(string)
				}
				if cmd, ok := specs["status"]; ok {
					statusCmd = cmd.(string)
				}
				if _regex, ok := specs["regex"]; ok {
					regex, err = regexp.Compile(_regex.(string))
					if err != nil {
						return err
					}
				}
			}
		case string:
			name = x
		default:
			return fmt.Errorf("Bad configuration of type %T in the `processes` section", x)
		}

		if statusCmd == "" {
			statusCmd = "name"
		}
		if user == "" {
			user = "root"
		}

		// fmt.Printf("Adding process %s to the list of monitored processes", name)

		MonitoredProcesses = append(MonitoredProcesses, &Process{
			Name:      name,
			Regex:     regex,
			StartCmd:  startCmd,
			StopCmd:   stopCmd,
			StatusCmd: statusCmd,
			User:      user,
		})
	}

	return nil
}
