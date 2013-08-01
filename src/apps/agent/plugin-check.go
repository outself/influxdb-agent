package main

import (
	log "code.google.com/p/log4go"
	"encoding/base64"
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

func checkNewPlugins() {
	log.Info("Checking for new plugins and for potentially useful plugins")

	for {
		plugins := getAvailablePlugins()

		// remove the plugins that are already running
		for _, plugin := range AgentConfig.Plugins {
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

func pluginShouldMonitorUrl(name string) string {
	return fmt.Sprintf("%s/%s/should_monitor", PLUGINS_REPO, name)
}

func pluginMetadataPath(name string) string {
	return fmt.Sprintf("%s/%s/meta.yml", PLUGINS_DIR, name)
}

func pluginMetadataUrl(name string) string {
	return fmt.Sprintf("%s/%s/meta.yml", PLUGINS_REPO, name)
}

func pluginStatusPath(name string) string {
	return fmt.Sprintf("%s/%s/status", PLUGINS_DIR, name)
}

func getAvailablePlugins() map[string]*Plugin {
	dirContent, err := getRemoteDirContent("/")
	if err != nil {
		log.Error("Error while getting list of plugins. %s", err)
		return nil
	}

	returnedPlugins := make(map[string]*Plugin)
	for _, plugin := range dirContent {
		if plugin.Type != "dir" {
			continue
		}

		plugin, err := downloadPluginBasic(plugin.Name)
		if err != nil {
			log.Error("Cannot download plugin script or metadata. %s", err)
			continue
		}
		returnedPlugins[plugin.Name] = plugin
	}

	return returnedPlugins
}

func downloadPluginBasic(name string) (*Plugin, error) {
	path := pluginShouldMonitorPath(name)
	url := pluginShouldMonitorUrl(name)

	log.Debug("Downloading should_monitor for %s", name)
	if err := getFileFromGithub(url, path); err != nil {
		return nil, fmt.Errorf("Error while getting the should_monitor script for plugin %s. %s", name, err)
	}

	path = pluginMetadataPath(name)
	url = pluginMetadataUrl(name)

	log.Debug("Downloading metadata for %s", name)
	if err := getFileFromGithub(url, path); err != nil {
		return nil, fmt.Errorf("Error while getting the metadata for plugin %s. %s", name, err)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Cannot open metadata file %s. Error: %s", path, err)
	}

	fileContent, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("Cannot read from file %s. Error: %s", path, err)
	}

	metadata := PluginMetadata{}
	if err := goyaml.Unmarshal(fileContent, &metadata); err != nil {
		return nil, fmt.Errorf("Cannot parse the metadata file. Content: '%s'. Error: %s", string(fileContent), err)
	}

	pluginCmd := pluginStatusPath(name)
	return &Plugin{pluginCmd, name, nil, metadata}, nil
}

func installPlugin(name string) error {
	plugin, err := downloadPluginBasic(name)
	if err != nil {
		return err
	}

	if plugin.Metadata.HasDependencies {
		return fmt.Errorf("Cannot install a pluign with dependencies")
	}

	files, err := getRemoteDirContent("/" + name)
	if err != nil {
		return err
	}
	for _, file := range files {
		// ignore these two files, we have them already
		if file.Name == "should_monitor" || file.Name == "meta.yml" {
			continue
		}

		url := fmt.Sprintf("%s/%s/%s", PLUGINS_REPO, name, file.Name)
		filePath := fmt.Sprintf("%s/%s/%s", PLUGINS_DIR, name, file.Name)
		if err := getFileFromGithub(url, filePath); err != nil {
			return err
		}
	}
	return nil
}

func getFileFromGithub(url, fullPath string) error {
	// create the plugins directory if it doesn't exist
	dir := path.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Error("Cannot create the plugin directory. Error: %s", err)
		return nil
	}

	// download the should_monitor script

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("Cannot get %s. Error: %s", url, err)
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Errorf("Cannot read %s. Error: %s", url, err)
	}

	parsedResponse := make(map[string]interface{})
	if err := json.Unmarshal(content, &parsedResponse); err != nil {
		fmt.Errorf("Cannot parse response '%s'. Error: %s", content, err)
	}

	//panic(string(content))

	base64Content := parsedResponse["content"].(string)
	data, err := base64.StdEncoding.DecodeString(base64Content)
	if err != nil {
		fmt.Errorf("Cannot base64 decode '%s'. Error: %s", base64Content, err)
	}

	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		fmt.Errorf("Cannot open %s. Error: %s", fullPath, err)
	}

	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		fmt.Errorf("Cannot write to %s. Error: %s", fullPath, err)
	}
	return nil
}

type DirContent struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func getRemoteDirContent(path string) ([]*DirContent, error) {
	url := fmt.Sprintf("%s%s", PLUGINS_REPO, path)
	resp, err := http.Get(url)

	if err != nil {
		return nil, fmt.Errorf("Cannot access plugins repo. Error: %s", err)
	}

	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)

	dirContents := make([]*DirContent, 0)
	err = json.Unmarshal(content, &dirContents)
	if err != nil {
		return nil, fmt.Errorf("Error while parsing repo contents. Error: %s", err)
	}

	return dirContents, nil
}
