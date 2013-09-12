package main

import (
	log "code.google.com/p/log4go"
	"github.com/errplane/errplane-go-common/agent"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"os/exec"
	"path"
	"time"
	. "utils"
)

func (self *Agent) sendPluginInfo(plugins map[string]*PluginMetadata, running []string) {
	info := &agent.AgentPluginInformation{}

	log.Info("Running plugins: %v\n", running)

	for _, name := range running {
		info.RunningPlugins = append(info.RunningPlugins, name)
	}

	for _, plugin := range plugins {
		if !plugin.IsCustom {
			continue
		}
		infoFile := path.Join(plugin.Path, "info.yml")
		infoContent, err := ioutil.ReadFile(infoFile)
		if err != nil {
			log.Error("Cannot read %s. Error: %s", infoFile, err)
			continue
		}
		pluginInfo := &agent.PluginInformationV2{}
		err = goyaml.Unmarshal(infoContent, info)
		if err != nil {
			log.Error("Cannot parse %s. Error: %s", infoContent, err)
			continue
		}
		pluginInfo.Name = plugin.Name
		info.CustomPlugins = append(info.CustomPlugins, pluginInfo)
	}

	if err := self.configClient.SendPluginInformation(info); err != nil {
		log.Error("Cannot send custom plugins information. Error: %s", err)
	}
}

func (self *Agent) checkNewPlugins() {
	log.Info("Checking for new plugins and for potentially useful plugins")

	for {
		plugins := self.getAvailablePlugins()

		disabledPlugins := make(map[string]bool)
		agentConfiguration, err := self.configClient.GetAgentConfiguration()
		if err == nil {
			for _, plugin := range agentConfiguration.DisabledPlugins {
				disabledPlugins[plugin] = true
			}
		}

		pluginsToCheck := make(map[string]*PluginMetadata)
		for name, plugin := range plugins {
			if disabledPlugins[name] {
				continue
			}
			pluginsToCheck[name] = plugin
		}

		availablePlugins := make([]string, 0)

		for name, plugin := range pluginsToCheck {
			log.Debug("checking whether plugin %s needs to be installed on this server or not", name)

			cmd := exec.Command(path.Join(plugin.Path, "should_monitor"))
			err := cmd.Run()
			if err != nil {
				log.Debug("Doesn't seem like %s is installed on this server. Error: %s.", name, err)
				continue
			}

			availablePlugins = append(availablePlugins, name)
			log.Debug("Plugin %s should be installed on this server. availablePlugins: %v", name, availablePlugins)
		}

		self.sendPluginInfo(plugins, availablePlugins)

		// update the agent information
		self.configClient.SendPluginStatus(&AgentStatus{availablePlugins, time.Now().Unix()})

		time.Sleep(self.config.Sleep)
	}
}

func (self *Agent) GetInstalledPluginsVersion() (string, error) {
	version, err := ioutil.ReadFile(path.Join(self.config.PluginsDir, "version"))
	if err != nil {
		return "", err
	}
	return string(version), nil
}

func (self *Agent) getAvailablePlugins() map[string]*PluginMetadata {
	version, err := self.GetInstalledPluginsVersion()
	if err != nil && !os.IsNotExist(err) {
		return nil
	}

	latestVersion, err := self.configClient.GetCurrentPluginsVersion()
	if err != nil {
		log.Error("Cannot current plugins version. Error: %s", err)
		return nil
	}

	if string(version) != string(latestVersion) {
		self.configClient.InstallPlugin(latestVersion)
	}

	pluginsDir := path.Join(self.config.PluginsDir, string(latestVersion))
	plugins, err := getPluginsInfo(pluginsDir)
	if err != nil {
		log.Error("Cannot list directory '%s'. Error: %s", pluginsDir, err)
		return nil
	}
	customPlugins, err := getPluginsInfo(self.config.CustomPluginsDir)
	if err != nil {
		log.Error("Cannot list directory '%s'. Error: %s", self.config.CustomPluginsDir, err)
		return nil
	}

	// custom plugins take precendence
	for name, info := range customPlugins {
		info.IsCustom = true
		plugins[name] = info
	}
	return plugins
}

func getPluginsInfo(dir string) (map[string]*PluginMetadata, error) {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	plugins := make(map[string]*PluginMetadata)
	for _, info := range infos {
		if !info.IsDir() {
			log.Debug("'%s' isn't a directory.Skipping!", info.Name())
			continue
		}

		dirname := info.Name()
		pluginDir := path.Join(dir, dirname)
		plugin, err := parsePluginInfo(pluginDir)
		if err != nil {
			log.Error("Cannot parse directory '%s'. Error: %s", dirname, err)
			continue
		}
		plugins[plugin.Name] = plugin
		plugin.Path = pluginDir
	}
	return plugins, nil
}

func parsePluginInfo(dirname string) (*PluginMetadata, error) {
	info := path.Join(dirname, "info.yml")

	infoContent, err := ioutil.ReadFile(info)
	if err != nil {
		return nil, err
	}

	metadata := PluginMetadata{}
	if err := goyaml.Unmarshal(infoContent, &metadata); err != nil {
		return nil, err
	}
	metadata.Name = path.Base(dirname)
	metadata.Path = dirname

	return &metadata, nil
}
