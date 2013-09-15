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

type AgentStatus struct {
	Plugins   []string `json:"plugins"`
	Timestamp int64    `json:"timestamp"`
}

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

func (self *ConfigServiceClient) GetMonitoringConfig() (*monitoring.MonitorConfig, error) {
	database := self.config.Database()
	hostname := self.config.Hostname
	apiKey := self.config.ApiKey

	if self.config.Hostname == "" {
		return nil, fmt.Errorf("Configuration service hostname not configured properly")
	}

	url := self.configServerUrl("/v2/databases/%s/agents/%s/monitoring_config?api_key=%s", database, hostname, apiKey)
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

func (self *ConfigServiceClient) GetAgentConfiguration() (*agent.AgentConfiguration, error) {
	database := self.config.Database()
	url := self.configServerUrl("/v2/databases/%s/agents/%s/configuration?api_key=%s", database, self.config.Hostname, self.config.ApiKey)
	body, err := GetBody(url)
	if err != nil {
		return nil, err
	}
	config := &agent.AgentConfiguration{}
	err = json.Unmarshal(body, config)
	return config, err
}

func (self *ConfigServiceClient) GetMonitoredProcesses() ([]*monitoring.ProcessMonitor, error) {
	config, err := self.GetAgentConfiguration()
	if err != nil {
		return nil, err
	}

	monitors := config.ProcessMonitors
	for _, process := range monitors {
		if process.User == "" {
			process.User = "root"
		}

		if process.Start == "" {
			process.Start = fmt.Sprintf("service %s start", process.Name)
		}

		if process.Stop == "" {
			process.Stop = "kill"
		}
	}

	return monitors, nil
}
