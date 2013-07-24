package main

import (
	log "code.google.com/p/log4go"
	"github.com/errplane/errplane-go"
	"github.com/errplane/gosigar"
	"math"
	"os/exec"
	"strings"
	"time"
)

type Status int

const (
	UP Status = iota
	DOWN
)

type Process struct {
	name       string
	startCmd   string
	stopCmd    string
	statusCmd  string
	user       string
	lastStatus Status
}

func monitorProceses(ep *errplane.Errplane, monitoredProcesses []*Process, ch chan error) {
	pids := sigar.ProcList{}

	var previousProcessesSnapshot map[string]ProcStat
	var previousProcessesSnapshotByPid map[int]ProcStat

	monitoringSleep := math.Max(float64(sleep/10), 1)

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

			if err := state.Get(pid); err != nil {
				log.Error("Cannot retrieve stat of pid %d", pid)
				continue
			}

			// FIXME: what if there is more than one process ?
			processes[state.Name] = *procStat
			processesByPid[pid] = *procStat
		}

		if previousProcessesSnapshot != nil {

			for _, monitoredProcess := range monitoredProcesses {
				status := getProcessStatus(monitoredProcess, processes)

				if status != monitoredProcess.lastStatus {
					if status == UP {
						reportProcessUp(ep, monitoredProcess)
					} else {
						// holy shit, process down!
						reportProcessDown(ep, monitoredProcess)
					}
				}
				monitoredProcess.lastStatus = status
				// process is still up, or is still down. Do nothing in both cases.
			}

			// report the cpu usage and memory usage
			mergedStats := mergeStats(previousProcessesSnapshotByPid, processesByPid)
			for _, stat := range mergedStats {
				reportProcessCpuUsage(ep, &stat, now, ch)
				reportProcessMemUsage(ep, &stat, now, ch)
			}
		}

		previousProcessesSnapshot = processes
		previousProcessesSnapshotByPid = processesByPid

		time.Sleep(time.Duration(monitoringSleep))
	}
}

func getProcessStatus(process *Process, currentProcessesSnapshot map[string]ProcStat) Status {
	if process.statusCmd == "ps" {
		state, ok := currentProcessesSnapshot[process.name]

		log.Fine("Getting status of %s, %v, %v", process.name, state, ok)

		if ok {
			return UP
		}
		return DOWN
	}

	log.Error("Unknown status command '%s' used. Assuming process down", process.statusCmd)
	return DOWN
}

func reportProcessDown(ep *errplane.Errplane, process *Process) {
	log.Info("Process %s went down, restarting and reporting event", process.name)
	reportProcessEvent(ep, process.name, "down")

	// The following requires an entry like the following in the sudoers file
	// errplane ALL=(root) NOPASSWD: /usr/sbin/service mysql start, (root) NOPASSWD: /usr/sbin/service mysql stop
	// where root is the user that is used to start and stop the service

	args := []string{"-u", process.user, "-n"}
	args = append(args, strings.Fields(process.startCmd)...)
	cmd := exec.Command("sudo", args...)
	if err := cmd.Run(); err != nil {
		log.Error("Error while starting service %s. Error: %s", process.name, err)
	}
}

func reportProcessUp(ep *errplane.Errplane, process *Process) {
	log.Info("Process %s came back up reporting event", process.name)
	reportProcessEvent(ep, process.name, "up")
}

func reportProcessEvent(ep *errplane.Errplane, name, status string) {
	ep.Report("server.process.monitoring", 1.0, time.Now(), "", errplane.Dimensions{
		"host":    hostname,
		"process": name,
		"status":  status,
	})
}
