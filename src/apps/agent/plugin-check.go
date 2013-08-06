package main

import (
	"bytes"
	log "code.google.com/p/log4go"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"net/http"
	"os"
	"os/exec"
	"path"
	"time"
	. "utils"
)

const (
	PLUGINS_REPO = "https://api.github.com/repos/errplane/errplane-plugins/contents"
	PLUGINS_DIR  = "/data/errplane-agent/plugins"
)

type AgentInformation struct {
	Plugins []string `json:"plugins"`
}

func checkNewPlugins() {
	log.Info("Checking for new plugins and for potentially useful plugins")

	for {
		plugins := getAvailablePlugins()

		// remove the plugins that are already running
		for _, plugin := range AgentConfig.Plugins {
			delete(plugins, plugin.Name)
		}

		availablePlugins := make([]string, 0)

		for name, plugin := range plugins {
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

		// update the agent information
		data, err := json.Marshal(AgentInformation{availablePlugins})
		if err == nil {
			database := AgentConfig.AppKey + AgentConfig.Environment
			url := fmt.Sprintf("http://%s/databases/%s/agent/%s?api_key=%s", AgentConfig.ConfigService, database,
				AgentConfig.Hostname, AgentConfig.ApiKey)
			log.Debug("posting to '%s' -- %s", url, data)
			resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
			if err != nil {
				log.Error("Cannot post agent information to '%s'. Error: %s", url, err)
			} else {
				defer resp.Body.Close()
			}
		} else {
			log.Error("Cannot marshal data to json")
		}

		time.Sleep(AgentConfig.Sleep)
	}
}

func getBody(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		log.Error("Cannot download from '%s'. Error: %s", url, err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Received status code %d", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func getAvailablePlugins() map[string]*PluginMetadata {
	version, err := ioutil.ReadFile(path.Join(PLUGINS_DIR, "version"))
	if err != nil && !os.IsNotExist(err) {
		return nil
	}

	database := AgentConfig.AppKey + AgentConfig.Environment
	url := fmt.Sprintf("http://%s/databases/%s/plugins/current_version", AgentConfig.ConfigService, database)
	latestVersion, err := getBody(url)
	if err != nil {
		log.Error("Cannot current plugins version from '%s'. Error: %s", url, err)
		return nil
	}

	if string(version) != string(latestVersion) {
		url = fmt.Sprintf("http://%s/databases/%s/plugins/%s", AgentConfig.ConfigService, database, latestVersion)
		plugins, err := getBody(url)
		if err != nil {
			log.Error("Cannot download plugin version from url '%s'. Error: %s", url, err)
			return nil
		}

		filename := path.Join(PLUGINS_DIR, string(latestVersion)+".tar.gz")
		if err := ioutil.WriteFile(filename, plugins, 0644); err != nil {
			log.Error("Cannot write to %s. Error: %s", filename, err)
			return nil
		}
		versionFilename := path.Join(PLUGINS_DIR, "version")
		if err := ioutil.WriteFile(versionFilename, latestVersion, 0644); err != nil {
			log.Error("Cannot write to %s. Error: %s", filename, err)
			return nil
		}

		dir := path.Join(PLUGINS_DIR, string(latestVersion))
		err = os.Mkdir(dir, 0755)
		if err != nil {
			log.Error("Cannot create directory '%s'", dir)
			return nil
		}
		cmd := exec.Command("tar", "-xvzf", filename)
		cmd.Dir = dir
		err = cmd.Run()
		if err != nil {
			log.Error("Cannot extract %s. Error: %s", filename, err)
			return nil
		}
	}

	pluginsDir := path.Join(PLUGINS_DIR, string(latestVersion))
	infos, err := ioutil.ReadDir(pluginsDir)
	if err != nil {
		log.Error("Cannot list directory '%s'. Error: %s", pluginsDir, err)
		return nil
	}

	plugins := make(map[string]*PluginMetadata)
	for _, info := range infos {
		if !info.IsDir() {
			log.Debug("'%s' isn't a directory.Skipping!", info.Name())
			continue
		}

		dirname := info.Name()
		pluginDir := path.Join(pluginsDir, dirname)
		plugin, err := parsePluginInfo(pluginDir)
		if err != nil {
			log.Error("Cannot parse directory '%s'. Error: %s", dirname, err)
			continue
		}
		plugins[plugin.Name] = plugin
		plugin.Path = pluginDir
	}
	return plugins
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
