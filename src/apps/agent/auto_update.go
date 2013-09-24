package main

import (
	log "code.google.com/p/log4go"
	"fmt"
	"os"
	"os/exec"
	"syscall"
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
			log.Debug("Same version, sleeping")
			continue
		}

		// if a newer is available install the new version, (run the curl command)
		log.Info("Updating from version %s to version %s", self.config.Version, newVersion)

		cmdString := fmt.Sprintf("curl https://getanomalous.com/install.sh | bash -s %s %s %s off",
			self.config.AppKey, self.config.ApiKey, newVersion)
		log.Debug("Running %s", cmdString)
		cmd := exec.Command("sh", "-c", cmdString)
		output, err := cmd.CombinedOutput()

		if err != nil {
			log.Error("Failed to update the agent. Output: %s, Error: %s", output, err)
			continue
		}

		// otherwise do an exec
		err = syscall.Exec(os.Args[0], os.Args, os.Environ())
		if err != nil {
			log.Error("Failed to restart the agent. Error: %s", err)
		}
	}
}
