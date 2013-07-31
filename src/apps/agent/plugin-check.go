package main

import (
	log "code.google.com/p/log4go"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"time"
	. "utils"
)

const (
	PLUGINS_REPO = "https://api.github.com/repos/errplane/errplane-plugins/contents/"
	PLUGINS_DIR  = "/data/errplane-agent/plugins"
)

func checkNewPlugins() {
	log.Info("Checking for new plugins and for potentially useful plugins")

	for {
		plugins := getOnlinePlugins()

		// remove the plugins that are already running
		for _, plugin := range Plugins {
			delete(plugins, plugin.Name)
		}

		for name, _ := range plugins {
			log.Debug("checking whether plugin %s needs to be installed on this server or not", name)

			cmd := exec.Command(fmt.Sprintf("%s/%s/should_monitor", PLUGINS_DIR, name))
			err := cmd.Run()
			if err != nil {
				log.Debug("Doesn't seem like %s is installed on this server. Error: %s.", name, err)
				continue
			}

			// FIXME: where should we report that the service can be monitored
			log.Debug("Plugin %s should be installed on this server.", name)
		}

		time.Sleep(1 * time.Hour)
	}
}

func pluginShouldMonitorPath(name string) string {
	return fmt.Sprintf("%s/%s/should_monitor", PLUGINS_DIR, name)
}

func getOnlinePlugins() map[string]*Plugin {
	resp, err := http.Get(PLUGINS_REPO)

	if err != nil {
		log.Error("Cannot access plugins repo")
		return nil
	}

	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)

	type RepoPlugin struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}

	repoPlugins := make([]*RepoPlugin, 0)
	err = json.Unmarshal(content, &repoPlugins)
	if err != nil {
		log.Error("Error while parsing repo contents. Error: %s", err)
		return nil
	}

	returnedPlugins := make(map[string]*Plugin)

	for _, plugin := range repoPlugins {
		if plugin.Type != "dir" {
			continue
		}

		log.Debug("Downloading should_monitor for %s", plugin.Name)

		// create the plugins directory if it doesn't exist
		if err := os.MkdirAll(fmt.Sprintf("%s/%s", PLUGINS_DIR, plugin.Name), 0755); err != nil {
			log.Error("Cannot create the plugin directory. Error: %s", err)
			return nil
		}

		// download the should_monitor script
		pluginShouldMonitorScript := fmt.Sprintf("%s%s/should_monitor", PLUGINS_REPO, plugin.Name)
		// panic(pluginShouldMonitorScript)

		resp, err := http.Get(pluginShouldMonitorScript)
		if err != nil {
			log.Error("Cannot get the should_monitor script of the %s plugin", plugin.Name)
			continue
		}
		defer resp.Body.Close()

		content, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Error("Cannot read the should_monitor script of the %s plugin", plugin.Name)
			continue
		}

		parsedResponse := make(map[string]interface{})
		if err := json.Unmarshal(content, &parsedResponse); err != nil {
			log.Error("Cannot parse response. Error: %s", err)
			continue
		}

		//panic(string(content))

		base64Content := parsedResponse["content"].(string)
		data, err := base64.StdEncoding.DecodeString(base64Content)
		if err != nil {
			log.Error("Cannot base64 decode the content of the should_monitor script. Error: %s", err)
			continue
		}

		file, err := os.OpenFile(pluginShouldMonitorPath(plugin.Name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
		if err != nil {
			log.Error("Cannot open the should_monitor file")
			continue
		}

		defer file.Close()

		_, err = file.Write(data)
		if err != nil {
			log.Error("Cannot write to %s", file.Name())
			continue
		}

		pluginCmd := fmt.Sprintf("%s/%s/status", PLUGINS_DIR, plugin.Name)
		returnedPlugins[plugin.Name] = &Plugin{pluginCmd, plugin.Name, nil}
	}

	return returnedPlugins
}
