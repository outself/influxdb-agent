package main

import (
	log "code.google.com/p/log4go"
	"fmt"
	"github.com/errplane/gosigar"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"utils"
)

type ProcsByName map[string]*ProcStat
type ProcsByPid map[int]*ProcStat

func getProcesses() (ProcsByName, ProcsByPid) {
	processes := make(map[string]*ProcStat)
	processesByPid := make(map[int]*ProcStat)

	pids := sigar.ProcList{}
	pids.Get()

	for _, pid := range pids.List {
		procStat, err := getProcStat(pid)
		if err != nil {
			log.Warn("Cannot get stat for pid %d. Error: %s", pid, err)
			continue
		}

		state := procStat.state

		// FIXME: what if there is more than one process ?
		processes[state.Name] = procStat
		processesByPid[pid] = procStat
	}

	return processes, processesByPid
}

func (self *Agent) monitorProceses(ch chan error) {

	var previousProcessesSnapshot map[string]*ProcStat
	var previousProcessesSnapshotByPid map[int]*ProcStat

	var monitoredProcesses []*utils.Process

	for {
		// get the list of monitored processes from the config service
		var err error
		monitoredProcesses, err = self.configClient.GetMonitoredProcesses(monitoredProcesses)
		if err != nil {
			log.Error("Error while getting the list of processes to monitor. Error: %s", err)
		}

		processes, processesByPid := getProcesses()

		now := time.Now()

		if previousProcessesSnapshot != nil {

			for _, monitoredProcess := range monitoredProcesses {
				log.Debug("Checking process health %#v", monitoredProcess)

				status := getProcessStatus(monitoredProcess, processesByPid)

				if status != monitoredProcess.LastStatus {
					if status == utils.UP {
						self.reportProcessUp(monitoredProcess)
					} else {
						// holy shit, process down!
						self.reportProcessDown(monitoredProcess)
					}
				}

				if status == utils.DOWN {
					if _, ok := snoozedProcesses.Get(monitoredProcess.Name); !ok {
						startProcess(monitoredProcess)
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
						self.reportProcessCpuUsage(monitoredProcess, &stat, now, false, ch)
						self.reportProcessMemUsage(monitoredProcess, &stat, now, false, ch)
					}
				}
			}
		}

		previousProcessesSnapshot = processes
		previousProcessesSnapshotByPid = processesByPid

		time.Sleep(self.config.MonitoredSleep)
	}
}

func processMatches(monitoredProcess *utils.Process, process interface{}) bool {
	name := ""
	args := []string{}

	switch x := process.(type) {
	case MergedProcStat:
		name = x.name
		args = x.args
	case *ProcStat:
		name = x.state.Name
		args = x.args.List
	default:
		panic(fmt.Errorf("Unknwon type %T", process))
	}

	if len(args) == 0 || monitoredProcess.StatusCmd == "name" {
		return name == monitoredProcess.Name
	} else if monitoredProcess.StatusCmd == "regex" {
		fullCmd := strings.Join(args, " ")
		log.Debug("Matching %s to %s", fullCmd, monitoredProcess.Regex)
		matches, err := regexp.MatchString(monitoredProcess.Regex, fullCmd)
		if err != nil {
			log.Error("Cannot match regex. Error: %s", err)
			return false
		}
		return matches
	}
	return false
}

func findProcess(process *utils.Process, processes ProcsByPid) *ProcStat {
	for _, proc := range processes {
		if processMatches(process, proc) {
			return proc
		}
	}

	return nil
}

func getProcessStatus(process *utils.Process, currentProcessesSnapshot ProcsByPid) utils.Status {
	if process := findProcess(process, currentProcessesSnapshot); process != nil {
		return utils.UP
	}
	return utils.DOWN
}

func (self *Agent) reportProcessDown(process *utils.Process) {
	log.Info("Process %s went down", process.Name)
	self.reportProcessEvent(process, process.Regex, "down")
}

func runCmd(cmd, user string) error {
	args := []string{"-u", user, "-n"}
	args = append(args, strings.Fields(cmd)...)
	log.Info("Executing 'sudo -u %s -n %s'", user, cmd)
	command := exec.Command("sudo", args...)
	return command.Run()
}

func startProcess(process *utils.Process) {
	if process.StartCmd == "" {
		log.Warn("No start command found for service %s", process.Name)
	}

	if err := runCmd(process.StartCmd, process.User); err != nil {
		log.Error("Error while starting service %s. Error: %s", process.Name, err)
	}
}

func stopProcess(process *utils.Process) {
	log.Info("Trying to stop process %s", process.Name)

	if process.StopCmd == "kill" || process.StopCmd == "" {
		killProcess(process)
		return
	}

	if err := runCmd(process.StopCmd, process.User); err != nil {
		log.Error("Error while stopping service %s. Error: %s", process.Name, err)
	}
}

func killProcess(process *utils.Process) {
	_, processes := getProcesses()
	stat := findProcess(process, processes)
	if stat == nil {
		log.Warn("Cannot find process %s", process.Name)
		return
	}
	command := fmt.Sprintf("kill %d", stat.pid)
	log.Debug("Running: %s", command)
	if err := runCmd(command, process.Name); err == nil {
		return
	}
	command = fmt.Sprintf("kill -9 %d", stat.pid)
	log.Warn("Cannot kill process '%s', trying %s", process.Name, command)
	if err := runCmd(command, process.Name); err == nil {
		return
	}
	log.Error("Couldn't kill process '%s'", process.Name)
}

func (self *Agent) reportProcessUp(process *utils.Process) {
	log.Info("Process %s came back up reporting event", process.Name)
	self.reportProcessEvent(process, process.Regex, "up")
}

func (self *Agent) reportProcessEvent(process *utils.Process, regex, status string) {
	if _, ok := snoozedProcesses.Get(process.Nickname); ok {
		log.Debug("Not reporting %s event for '%s' since it is snoozed", status, process.Nickname)
		return
	}

	metricName := self.getServerStatMetricName(fmt.Sprintf("processes.%s.status", process.Nickname))
	self.Report(metricName, 1.0, time.Now(), status, nil)
}
