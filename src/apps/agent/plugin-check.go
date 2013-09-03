package main

import (
	log "code.google.com/p/log4go"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"os/exec"
	"path"
	"syscall"
	"time"
	. "utils"
)

func checkNewPlugins() {
	log.Info("Checking for new plugins and for potentially useful plugins")

	for {
		plugins := getAvailablePlugins()
		disabledPlugins := make(map[string]bool)
		config, err := GetPluginsToRun()
		if err != nil {
			log.Error("Error while getting configuration from backend. Error: %s", err)
		} else {
			for _, pluginName := range config.DisabledPlugins {
				disabledPlugins[pluginName] = true
			}
		}

		// all plugins are enabled

		// // remove the plugins that are already running

		availablePlugins := make(map[string]string)

		for name, plugin := range plugins {
			// log.Debug("checking whether plugin %s needs to be installed on this server or not", name)

			if _, ok := disabledPlugins[name]; ok {
				availablePlugins[name] = "disabled"
				continue
			}
			cmd := exec.Command(path.Join(plugin.Path, "should_monitor"))
			err := cmd.Run()
			if err != nil {
				availablePlugins[name] = "not_installed"
				log.Debug("Doesn't seem like %s is installed on this server. Error: %s.", name, err)
				continue
			}
			cmd = exec.Command(path.Join(plugin.Path, "status"))
			cmd.Run()
			exitStatus := cmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus()
			availablePlugins[name] = PluginStateOutput(exitStatus).String()

			// log.Debug("Plugin %s should be installed on this server. availablePlugins: %v", name, availablePlugins)
		}

		// update the agent information
		SendPluginStatus(&AgentStatus{availablePlugins, time.Now().Unix()})

		time.Sleep(AgentConfig.Sleep)
	}
}

func getAvailablePlugins() map[string]*PluginMetadata {
	version, err := GetInstalledPluginsVersion()
	if err != nil && !os.IsNotExist(err) {
		return nil
	}

	latestVersion, err := GetCurrentPluginsVersion()
	if err != nil {
		log.Error("Cannot current plugins version. Error: %s", err)
		return nil
	}

	if string(version) != string(latestVersion) {
		InstallPlugin(latestVersion)
	}

	pluginsDir := path.Join(PLUGINS_DIR, string(latestVersion))
	plugins, err := getPluginsInfo(pluginsDir)
	if err != nil {
		log.Error("Cannot list directory '%s'. Error: %s", pluginsDir, err)
		return nil
	}
	customPlugins, err := getPluginsInfo(CUSTOM_PLUGINS_DIR)
	if err != nil {
		log.Error("Cannot list directory '%s'. Error: %s", CUSTOM_PLUGINS_DIR, err)
		return nil
	}

	// report these plugins to the config api to be shown to the user on the UI
	if len(customPlugins) > 0 {
		customPluginsInfo := make(map[string]*PluginInformation)
		for name, plugin := range customPlugins {
			infoFile := path.Join(plugin.Path, "info.yml")
			infoContent, err := ioutil.ReadFile(infoFile)
			if err != nil {
				log.Error("Cannot read %s. Error: %s", infoFile, err)
				continue
			}
			info := &PluginInformation{}
			err = goyaml.Unmarshal(infoContent, info)
			if err != nil {
				log.Error("Cannot parse %s. Error: %s", infoContent, err)
				continue
			}
			customPluginsInfo[name] = info
		}

		if err := SendCustomPlugins(customPluginsInfo); err != nil {
			log.Error("Cannot send custom plugins information. Error: %s", err)
		}
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
