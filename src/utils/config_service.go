package utils

import (
	"bytes"
	log "code.google.com/p/log4go"
	"encoding/json"
	"fmt"
	"github.com/errplane/errplane-go-common/agent"
	"github.com/errplane/errplane-go-common/monitoring"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
)

type AgentConfiguration struct {
	Plugins   map[string][]*Instance `json:"plugins"`
	Processes []*Process             `json:"processes"`
}

type AgentStatus struct {
	Plugins   []string `json:"plugins"`
	Timestamp int64    `json:"timestamp"`
}

var AgentInfo *AgentConfiguration

type ConfigServiceClient struct {
	config *Config
}

func NewConfigServiceClient(config *Config) *ConfigServiceClient {
	return &ConfigServiceClient{
		config: config,
	}
}

// assume that path starts with /
func (self *ConfigServiceClient) configServerUrl(path string, args ...interface{}) string {
	separator := ""
	if path[0] != '/' {
		separator = "/"
	}

	if len(args) > 0 {
		path = fmt.Sprintf(path, args...)
	}

	return fmt.Sprintf("http://%s%s%s", self.config.ConfigService, separator, path)
}

func (self *ConfigServiceClient) SendPluginInformation(info *agent.AgentPluginInformation) error {
	data, err := json.Marshal(info)
	if err != nil {
		log.Error("Cannot marshal data to json")
		return err
	}
	database := self.config.Database()
	hostname := self.config.Hostname
	apiKey := self.config.ApiKey
	url := self.configServerUrl("/v2/databases/%s/agents/%s?api_key=%s", database, hostname, apiKey)
	log.Debug("posting to '%s' -- %s", url, data)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Error("Cannot post running plugins info to '%s'. Error: %s", url, err)
		return err
	}
	resp.Body.Close()
	return nil
}

func (self *ConfigServiceClient) SendPluginStatus(status *AgentStatus) {
	data, err := json.Marshal(status)
	if err != nil {
		log.Error("Cannot marshal data to json")
		return
	}
	database := self.config.Database()
	hostname := self.config.Hostname
	apiKey := self.config.ApiKey
	url := self.configServerUrl("/databases/%s/agent/%s?api_key=%s", database, hostname, apiKey)
	log.Debug("posting to '%s' -- %s", url, data)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Error("Cannot post agent information to '%s'. Error: %s", url, err)
		return
	}
	resp.Body.Close()
}

func (self *ConfigServiceClient) GetMonitoringConfig() (*monitoring.MonitorConfig, error) {
	database := self.config.Database()
	hostname := self.config.Hostname
	apiKey := self.config.ApiKey

	if self.config.Hostname == "" {
		return nil, fmt.Errorf("Configuration service hostname not configured properly")
	}

	url := self.configServerUrl("/databases/%s/agent/%s/monitoring-configuration?api_key=%s", database, hostname, apiKey)
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

func (self *ConfigServiceClient) InstallPlugin(version string) {
	database := self.config.Database()
	url := self.configServerUrl("/databases/%s/plugins/%s", database, version)
	plugins, err := GetBody(url)
	if err != nil {
		log.Error("Cannot download plugin version from url '%s'. Error: %s", url, err)
		return
	}

	filename := path.Join(self.config.PluginsDir, version+".tar.gz")
	if err := ioutil.WriteFile(filename, plugins, 0644); err != nil {
		log.Error("Cannot write to %s. Error: %s", filename, err)
		return
	}
	versionFilename := path.Join(self.config.PluginsDir, "version")
	if err := ioutil.WriteFile(versionFilename, []byte(version), 0644); err != nil {
		log.Error("Cannot write to %s. Error: %s", filename, err)
		return
	}

	dir := path.Join(self.config.PluginsDir, version)
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

func (self *ConfigServiceClient) GetCurrentPluginsVersion() (string, error) {
	database := self.config.Database()
	url := self.configServerUrl("/databases/%s/plugins/current_version", database)
	version, err := GetBody(url)
	if err != nil {
		return "", err
	}
	return string(version), nil
}

func (self *ConfigServiceClient) GetMonitoredProcesses(processes []*Process) ([]*Process, error) {
	config, err := self.GetPluginsToRun()
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

func (self *ConfigServiceClient) GetPluginsToRun() (*AgentConfiguration, error) {
	config := &AgentConfiguration{}
	database := self.config.Database()
	hostname := self.config.Hostname
	apiKey := self.config.ApiKey
	url := self.configServerUrl("/databases/%s/agent/%s/configuration?api_key=%s", database, hostname, apiKey)
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
