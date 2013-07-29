package main

import (
	log "code.google.com/p/log4go"
	"fmt"
	"github.com/errplane/errplane-go"
	"github.com/errplane/gosigar"
	"math"
	"os/exec"
	"strings"
	"time"
	. "utils"
)

func monitorProceses(ep *errplane.Errplane, monitoredProcesses []*Process, ch chan error) {
	pids := sigar.ProcList{}

	var previousProcessesSnapshot map[string]ProcStat
	var previousProcessesSnapshotByPid map[int]ProcStat

	monitoringSleep := math.Max(float64(Sleep/10), 1)

	for {
		pids.Get()

		processes := make(map[string]ProcStat)
		processesByPid := make(map[int]ProcStat)

		now := time.Now()

		for _, pid := range pids.List {
			procStat := getProcStat(pid)

			if procStat == nil {
				log.Warn("Cannot get stat for pid %d", pid)
				continue
			}

			state := procStat.state

			// FIXME: what if there is more than one process ?
			processes[state.Name] = *procStat
			processesByPid[pid] = *procStat
		}

		if previousProcessesSnapshot != nil {

			for _, monitoredProcess := range monitoredProcesses {
				status := getProcessStatus(monitoredProcess, processes)

				if status != monitoredProcess.LastStatus {
					if status == UP {
						reportProcessUp(ep, monitoredProcess)
					} else {
						// holy shit, process down!
						reportProcessDown(ep, monitoredProcess)
					}
				}
				monitoredProcess.LastStatus = status
				// process is still up, or is still down. Do nothing in both cases.
			}

			// report the cpu usage and memory usage
			mergedStats := mergeStats(previousProcessesSnapshotByPid, processesByPid)
			i := 0
			for _, stat := range mergedStats {
				for _, monitoredProcess := range monitoredProcesses {
					if processMatches(monitoredProcess, stat) {
						i += 1
						reportProcessCpuUsage(ep, &stat, now, ch)
						reportProcessMemUsage(ep, &stat, now, ch)
					}
				}
			}
		}

		previousProcessesSnapshot = processes
		previousProcessesSnapshotByPid = processesByPid

		time.Sleep(time.Duration(monitoringSleep))
	}
}

func processMatches(monitoredProcess *Process, process interface{}) bool {
	name := ""
	args := []string{}

	switch x := process.(type) {
	case MergedProcStat:
		name = x.name
		args = x.args
	case ProcStat:
		name = x.state.Name
		args = x.args.List
	default:
		panic(fmt.Errorf("Unknwon type %T", process))
	}

	if monitoredProcess.StatusCmd == "name" {
		return name == monitoredProcess.Name
	} else if monitoredProcess.StatusCmd == "regex" {
		_fullCmd := append([]string{name}, args...)
		fullCmd := strings.Join(_fullCmd, " ")
		return monitoredProcess.Regex.MatchString(fullCmd)
	}
	return false
}

func getProcessStatus(process *Process, currentProcessesSnapshot map[string]ProcStat) Status {
	if process.StatusCmd == "name" {
		state, ok := currentProcessesSnapshot[process.Name]

		log.Fine("Getting status of %s, %v, %v", process.Name, state, ok)

		if ok {
			return UP
		}
		return DOWN
	} else if process.StatusCmd == "regex" {
		found := false

		for _, proc := range currentProcessesSnapshot {
			if processMatches(process, proc) {
				found = true
				break
			}
		}

		if !found {
			return DOWN
		}
		return UP
	}

	log.Error("Unknown status command '%s' used. Assuming process down", process.StatusCmd)
	return DOWN
}

func reportProcessDown(ep *errplane.Errplane, process *Process) {
	log.Info("Process %s went down, restarting and reporting event", process.Name)
	reportProcessEvent(ep, process.Name, "down")

	// The following requires an entry like the following in the sudoers file
	// errplane ALL=(root) NOPASSWD: /usr/sbin/service mysql start, (root) NOPASSWD: /usr/sbin/service mysql stop
	// where root is the user that is used to start and stop the service

	if process.StartCmd == "" {
		log.Warn("No start command found for service %s", process.Name)
	}

	args := []string{"-u", process.User, "-n"}
	args = append(args, strings.Fields(process.StartCmd)...)
	cmd := exec.Command("sudo", args...)
	if err := cmd.Run(); err != nil {
		log.Error("Error while starting service %s. Error: %s", process.Name, err)
	}
}

func reportProcessUp(ep *errplane.Errplane, process *Process) {
	log.Info("Process %s came back up reporting event", process.Name)
	reportProcessEvent(ep, process.Name, "up")
}

func reportProcessEvent(ep *errplane.Errplane, name, status string) {
	ep.Report("server.process.monitoring", 1.0, time.Now(), "", errplane.Dimensions{
		"host":    Hostname,
		"process": name,
		"status":  status,
	})
}
