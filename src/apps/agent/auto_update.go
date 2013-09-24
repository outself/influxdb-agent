package main

import (
	log "code.google.com/p/log4go"
	"fmt"
	"os/exec"
	"time"
)

func (self *Agent) autoUpdate() {
	for {
		time.Sleep(self.config.Sleep)
		// compare with the current agent version
		newVersion, err := self.configClient.GetAgentVersion()
		if err != nil {
			log.Debug("Cannot get agent version. Error: %s", err)
			continue
		}
		if newVersion == self.config.Version {
			continue
		}

		// if a newer is available install the new version, (run the curl command)
		log.Info("Updating from version %s to version %s", self.config.Version, newVersion)

		cmdString := fmt.Sprintf("sh", "-c", "curl https://getanomalous.com/install.sh | sudo bash -s %s %s", self.config.AppKey, self.config.ApiKey)
		cmd := exec.Command(cmdString)
		err = cmd.Run()
		if err != nil {
			log.Error("Failed to update the agent")
		}
	}
}
