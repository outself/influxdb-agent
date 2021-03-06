package utils

import (
	"bytes"
	log "code.google.com/p/log4go"
	"encoding/json"
	"fmt"
	"github.com/errplane/errplane-go-common/monitoring"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
)

const (
	PLUGINS_DIR        = "/data/errplane-agent/shared/plugins"
	CUSTOM_PLUGINS_DIR = "/data/errplane-agent/shared/custom-plugins"
)

type PluginInformation struct {
	BasicStats []struct {
		Name   string `json:"name"`
		Metric string `json:"metric"`
		Units  string `json:"units"`
	} `yaml:"basic-stats" json:"stats,omitempty"`
	Arguments []struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		DefaultValue string `yaml:"default_value" json:"default"`
	} `yaml:"arguments" json:"arguments,omitempty"`
}

type AgentConfiguration struct {
	Plugins   map[string][]*Instance `json:"plugins"`
	Processes []*Process             `json:"processes"`
}

type AgentStatus struct {
	Plugins   []string `json:"plugins"`
	Timestamp int64    `json:"timestamp"`
}

var AgentInfo *AgentConfiguration

// assume that path starts with /
func configServerUrl(path string, args ...interface{}) string {
	separator := ""
	if path[0] != '/' {
		separator = "/"
	}

	if len(args) > 0 {
		path = fmt.Sprintf(path, args...)
	}

	return fmt.Sprintf("http://%s%s%s", AgentConfig.ConfigService, separator, path)
}

func SendCustomPlugins(plugins map[string]*PluginInformation) error {
	data, err := json.Marshal(plugins)
	if err != nil {
		log.Error("Cannot marshal data to json")
		return err
	}
	database := AgentConfig.Database()
	hostname := AgentConfig.Hostname
	apiKey := AgentConfig.ApiKey
	url := configServerUrl("/databases/%s/agent/%s/custom-plugins?api_key=%s", database, hostname, apiKey)
	log.Debug("posting to '%s' -- %s", url, data)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Error("Cannot post agent information to '%s'. Error: %s", url, err)
		return err
	}
	resp.Body.Close()
	return nil
}

func SendPluginStatus(status *AgentStatus) {
	data, err := json.Marshal(status)
	if err != nil {
		log.Error("Cannot marshal data to json")
		return
	}
	database := AgentConfig.Database()
	hostname := AgentConfig.Hostname
	apiKey := AgentConfig.ApiKey
	url := configServerUrl("/databases/%s/agent/%s?api_key=%s", database, hostname, apiKey)
	log.Debug("posting to '%s' -- %s", url, data)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Error("Cannot post agent information to '%s'. Error: %s", url, err)
		return
	}
	resp.Body.Close()
}

func GetMonitoringConfig() (*monitoring.MonitorConfig, error) {
	database := AgentConfig.Database()
	hostname := AgentConfig.Hostname
	apiKey := AgentConfig.ApiKey

	if AgentConfig.Hostname == "" {
		return nil, fmt.Errorf("Configuration service hostname not configured properly")
	}

	url := configServerUrl("/databases/%s/agent/%s/monitoring-configuration?api_key=%s", database, hostname, apiKey)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Received status code %d", resp.StatusCode)
	}
	log.Debug("Received: %s", string(body))
	return monitoring.ParseMonitorConfig(string(body), false)
}

func GetInstalledPluginsVersion() (string, error) {
	version, err := ioutil.ReadFile(path.Join(PLUGINS_DIR, "version"))
	if err != nil {
		return "", err
	}
	return string(version), nil
}

func InstallPlugin(version string) {
	database := AgentConfig.Database()
	url := configServerUrl("/databases/%s/plugins/%s", database, version)
	plugins, err := GetBody(url)
	if err != nil {
		log.Error("Cannot download plugin version from url '%s'. Error: %s", url, err)
		return
	}

	filename := path.Join(PLUGINS_DIR, version+".tar.gz")
	if err := ioutil.WriteFile(filename, plugins, 0644); err != nil {
		log.Error("Cannot write to %s. Error: %s", filename, err)
		return
	}
	versionFilename := path.Join(PLUGINS_DIR, "version")
	if err := ioutil.WriteFile(versionFilename, []byte(version), 0644); err != nil {
		log.Error("Cannot write to %s. Error: %s", filename, err)
		return
	}

	dir := path.Join(PLUGINS_DIR, version)
	err = os.Mkdir(dir, 0755)
	if err != nil {
		log.Error("Cannot create directory '%s'", dir)
		return
	}
	cmd := exec.Command("tar", "-xvzf", filename)
	cmd.Dir = dir
	err = cmd.Run()
	if err != nil {
		log.Error("Cannot extract %s. Error: %s", filename, err)
		return
	}
}

func GetCurrentPluginsVersion() (string, error) {
	database := AgentConfig.Database()
	url := configServerUrl("/databases/%s/plugins/current_version", database)
	version, err := GetBody(url)
	if err != nil {
		return "", err
	}
	return string(version), nil
}

func GetMonitoredProcesses(processes []*Process) ([]*Process, error) {
	config, err := GetPluginsToRun()
	if err != nil {
		return nil, err
	}

	processesMap := make(map[string]*Process)
	for _, process := range processes {
		processesMap[process.Nickname] = process
	}

	returnedProcesses := make([]*Process, 0)

	for _, process := range config.Processes {
		if process.User == "" {
			process.User = "root"
		}

		if process.StartCmd == "" {
			process.StartCmd = fmt.Sprintf("service %s start", process.Nickname)
		}

		if p := processesMap[process.Nickname]; p != nil {
			process.LastStatus = p.LastStatus
		}
		returnedProcesses = append(returnedProcesses, process)
	}
	return returnedProcesses, nil
}

func GetPluginsToRun() (*AgentConfiguration, error) {
	config := &AgentConfiguration{}
	database := AgentConfig.Database()
	hostname := AgentConfig.Hostname
	apiKey := AgentConfig.ApiKey
	url := configServerUrl("/databases/%s/agent/%s/configuration?api_key=%s", database, hostname, apiKey)
	body, err := GetBody(url)
	if err != nil {
		return nil, err
	}
	log.Debug("Received configuration: %s", string(body))
	err = json.Unmarshal(body, config)
	if err != nil {
		return nil, err
	}
	log.Debug("Parsed response: %v", config)
	return config, nil
}
