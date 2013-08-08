package main

import (
	log "code.google.com/p/log4go"
	"github.com/bmizerany/pat"
	"github.com/pmylund/go-cache"
	"net/http"
	"time"
	. "utils"
)

// this file process local http request that contain commands to stop, start or restart a process
var snoozedProcesses *cache.Cache

func startLocalServer() {
	snoozedProcesses = cache.New(0, 0)

	m := pat.New()

	m.Get("/stop_monitoring/:process", http.HandlerFunc(stopMonitoring))
	m.Get("/start_monitoring/:process", http.HandlerFunc(startMonitoring))
	m.Get("/restart_process/:process", http.HandlerFunc(restartProcess))
}

func getProcess(processName string) (*Process, error) {
	monitoredProcesses, err := GetMonitoredProcesses(nil)
	if err != nil {
		return nil, err
	}

	// find the process and add it to our cache
	for _, process := range monitoredProcesses {
		if process.Name == processName {
			return process, nil
		}
	}
	return nil, nil
}

type InvalidProcessName struct{}

func (self *InvalidProcessName) Error() string {
	return "Invalid process name"
}

func snoozeProcess(processName string, duration time.Duration) error {
	process, err := getProcess(processName)
	if err != nil {
		return err
	}
	if process != nil {
		snoozedProcesses.Set(processName, true, duration)
		return nil
	}
	return &InvalidProcessName{}
}

func unsnoozeProcess(processName string) error {
	process, err := getProcess(processName)
	if err != nil {
		return err
	}
	if process != nil {
		snoozedProcesses.Delete(processName)
		return nil
	}
	return &InvalidProcessName{}
}

func stopMonitoring(w http.ResponseWriter, req *http.Request) {
	var err error
	// make sure that we're monitoring that process
	processName := req.URL.Query().Get("process")
	duration := req.URL.Query().Get("duration")
	timeDuration := time.Duration(-1)
	if duration != "" {
		timeDuration, err = time.ParseDuration(duration)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	if err := snoozeProcess(processName, timeDuration); err != nil {
		log.Warn("Error while snoozing process. Error: ", err)
		if _, ok := err.(*InvalidProcessName); ok {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

func startMonitoring(w http.ResponseWriter, req *http.Request) {
	processName := req.URL.Query().Get("process")

	if err := unsnoozeProcess(processName); err != nil {
		log.Warn("Error while unsnoozing process. Error: ", err)
		if _, ok := err.(*InvalidProcessName); ok {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

func restartProcess(w http.ResponseWriter, req *http.Request) {
	processName := req.URL.Query().Get("process")

	process, err := getProcess(processName)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	snoozedProcesses.Set(process.Name, true, -1)
	defer snoozedProcesses.Delete(process.Name)
	stopProcess(process)
	startProcess(process)
}
