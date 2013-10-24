package main

import (
	log "code.google.com/p/log4go"
	"flag"
	"fmt"
	"github.com/errplane/errplane-go"
	"github.com/errplane/gosigar"
	"io/ioutil"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	. "utils"
)

func main() {
	configFile := flag.String("config", "/etc/errplane-agent/config.yml", "The agent config file")
	flag.Parse()

	pidFile := flag.String("pidfile", "/data/errplane-agent/shared/errplane-agent.pid", "The agent pid file")
	flag.Parse()

	err := InitConfig(*configFile)
	if err != nil {
		fmt.Printf("Error while reading configuration. Error: %s", err)
		os.Exit(1)
	}

	err = initLog()
	if err != nil {
		fmt.Printf("Error while reading configuration. Error: %s", err)
		os.Exit(1)
	}

	if *pidFile == "" {
		fmt.Printf("Pidfile is a required argument and cannot be empty")
	}
	pid := os.Getpid()
	err = ioutil.WriteFile(*pidFile, []byte(strconv.Itoa(pid)), 0644)
	if err != nil {
		fmt.Printf("Error while writing to file %s. Error: %s", *pidFile, err)
	}

	ep := errplane.New(AgentConfig.AppKey, AgentConfig.Environment, AgentConfig.ApiKey)
	ep.SetHttpHost(AgentConfig.HttpHost)
	ep.SetUdpAddr(AgentConfig.UdpHost)
	if AgentConfig.Proxy != "" {
		ep.SetProxy(AgentConfig.Proxy)
	}
	ch := make(chan error)
	go memStats(ep, ch)
	go cpuStats(ep, ch)
	go networkStats(ep, ch)
	go diskSpaceStats(ep, ch)
	go ioStats(ep, ch)
	go procStats(ep, ch)
	go monitorProceses(ep, ch)
	go monitorPlugins(ep)
	go checkNewPlugins()
	go startUdpListener(ep)
	go startLocalServer()
	log.Info("Agent started successfully")
	err = <-ch
	log.Error("Data collection stopped unexpectedly. Error: %s", err)
	log.Close()
	time.Sleep(1 * time.Second) // give the logger a chance to close and write to the file
	return
}

func initLog() error {
	level := log.DEBUG
	switch AgentConfig.LogLevel {
	case "info":
		level = log.INFO
	case "warn":
		level = log.WARNING
	case "error":
		level = log.ERROR
	}

	log.AddFilter("file", level, log.NewFileLogWriter(AgentConfig.LogFile, false))

	var err error
	os.Stderr, err = os.OpenFile(AgentConfig.LogFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	os.Stdout = os.Stderr

	return nil
}

func report(ep *errplane.Errplane, metric string, value float64, timestamp time.Time, dimensions errplane.Dimensions, ch chan error) bool {
	err := ep.Report(metric, value, timestamp, "", dimensions)
	if err != nil {
		log.Error("Error while sending report. Error: %s", err)
	}
	return false
}

func procStats(ep *errplane.Errplane, ch chan error) {
	var previousStats map[int]*ProcStat

	for {
		_, procStats := getProcesses()

		if previousStats != nil {
			mergedStats := mergeStats(previousStats, procStats)

			n := int(math.Min(float64(AgentConfig.TopNProcesses), float64(len(mergedStats))))

			sort.Sort(ProcStatsSortableByCpu(mergedStats))
			topNByCpu := mergedStats[0:n]
			now := time.Now()
			for _, stat := range topNByCpu {
				if reportProcessCpuUsage(ep, nil, &stat, now, true, ch) {
					return
				}
			}
			sort.Sort(ProcStatsSortableByMem(mergedStats))
			topNByMem := mergedStats[0:n]
			for _, stat := range topNByMem {
				if reportProcessMemUsage(ep, nil, &stat, now, true, ch) {
					return
				}
			}
		}

		previousStats = procStats
		time.Sleep(AgentConfig.TopNSleep)
	}
}

func reportProcessCpuUsage(ep *errplane.Errplane, monitoredProcess *Process, stat *MergedProcStat, now time.Time, top bool, ch chan error) bool {
	return reportProcessMetric(ep, monitoredProcess, stat, "cpu", now, top, ch)
}

func reportProcessMemUsage(ep *errplane.Errplane, monitoredProcess *Process, stat *MergedProcStat, now time.Time, top bool, ch chan error) bool {
	return reportProcessMetric(ep, monitoredProcess, stat, "mem", now, top, ch)
}

func reportProcessMetric(ep *errplane.Errplane, monitoredProcess *Process, stat *MergedProcStat, metricName string, now time.Time, top bool, ch chan error) bool {
	var value float64
	var metric string

	suffix := ""
	if top {
		suffix = ".top"
	}

	switch metricName {
	case "cpu":
		metric = fmt.Sprintf("server.stats.procs.cpu%s", suffix)
		value = stat.cpuUsage
	case "mem":
		metric = fmt.Sprintf("server.stats.procs.mem%s", suffix)
		value = stat.memUsage
	default:
		log.Error("unknown metric name %s", metricName)
		return true
	}
	var dimensions errplane.Dimensions
	if monitoredProcess != nil {
		dimensions = errplane.Dimensions{
			"nickname": monitoredProcess.Nickname,
			"host":     AgentConfig.Hostname,
		}
	} else {
		dimensions = errplane.Dimensions{
			"pid":     strconv.Itoa(stat.pid),
			"name":    stat.name,
			"cmdline": strings.Join(stat.args, " "),
			"host":    AgentConfig.Hostname,
		}
	}

	if report(ep, metric, value, now, dimensions, ch) {
		return true
	}
	return false
}

func ioStats(ep *errplane.Errplane, ch chan error) {
	prevTimeStamp := time.Now()
	var prevDiskUsages []DiskUsage

	for {
		timestamp := time.Now()
		diskUsages, err := GetDiskUsages()
		if err != nil {
			ch <- err
			return
		}

		if prevDiskUsages != nil {
			devNameToDiskUsage := make(map[string]*DiskUsage)
			for idx, prevDiskUsage := range prevDiskUsages {
				devNameToDiskUsage[prevDiskUsage.Name] = &prevDiskUsages[idx]
			}

			for _, diskUsage := range diskUsages {
				prevDiskUsage := devNameToDiskUsage[diskUsage.Name]
				if prevDiskUsage == nil {
					log.Warn("Cannot find %s in previous disk usage", diskUsage.Name)
					continue
				}

				millisecondsElapsed := timestamp.Sub(prevTimeStamp).Nanoseconds() / int64(time.Millisecond)
				utilization := float64(diskUsage.TotalIOTime-prevDiskUsage.TotalIOTime) / float64(millisecondsElapsed) * 100

				dimensions := errplane.Dimensions{"host": AgentConfig.Hostname, "device": diskUsage.Name}

				if report(ep, "server.stats.io.utilization", float64(utilization), timestamp, dimensions, ch) {
					return
				}
			}
		}

		prevDiskUsages = diskUsages
		prevTimeStamp = timestamp
		time.Sleep(AgentConfig.Sleep)
	}
}

func memStats(ep *errplane.Errplane, ch chan error) {
	mem := sigar.Mem{}
	swap := sigar.Swap{}

	for {
		err := mem.Get()
		if err != nil {
			ch <- err
			return
		}
		err = swap.Get()
		if err != nil {
			ch <- err
			return
		}

		dimensions := errplane.Dimensions{"host": AgentConfig.Hostname}
		timestamp := time.Now()

		used := float64(mem.Used)
		actualUsed := float64(mem.ActualUsed)
		usedPercentage := actualUsed / float64(mem.Total) * 100

		if swap.Total > 0 {
			// report swap usage only if the server has swap enabled

			swapUsed := float64(swap.Used)
			swapUsedPercentage := swapUsed / float64(swap.Total) * 100

			if report(ep, "server.stats.swap.used", swapUsed, timestamp, dimensions, ch) ||
				report(ep, "server.stats.swap.used_percentage", swapUsedPercentage, timestamp, dimensions, ch) {
				return
			}
		}

		if report(ep, "server.stats.memory.free", float64(mem.Free), timestamp, dimensions, ch) ||
			report(ep, "server.stats.memory.used", used, timestamp, dimensions, ch) ||
			report(ep, "server.stats.memory.actual_used", actualUsed, timestamp, dimensions, ch) ||
			report(ep, "server.stats.memory.used_percentage", usedPercentage, timestamp, dimensions, ch) ||
			report(ep, "server.stats.swap.free", float64(swap.Free), timestamp, dimensions, ch) {
			return
		}

		time.Sleep(AgentConfig.Sleep)
	}
}

func diskSpaceStats(ep *errplane.Errplane, ch chan error) {
	fslist := sigar.FileSystemList{}

	for {
		fslist.Get()

		timestamp := time.Now()

		for _, fs := range fslist.List {
			dir_name := fs.DirName

			usage := sigar.FileSystemUsage{}
			usage.Get(dir_name)

			dimensions := errplane.Dimensions{"host": AgentConfig.Hostname, "device": fs.DevName}

			used := float64(usage.Total)
			usedPercentage := usage.UsePercent()

			if report(ep, "server.stats.disk.used", used, timestamp, dimensions, ch) ||
				report(ep, "server.stats.disk.used_percentage", usedPercentage, timestamp, dimensions, ch) {
				return
			}
		}
		time.Sleep(AgentConfig.Sleep)
	}
}

func cpuStats(ep *errplane.Errplane, ch chan error) {
	skipFirst := true

	prevCpu := sigar.Cpu{}
	cpu := sigar.Cpu{}

	for {
		timestamp := time.Now()
		err := cpu.Get()
		if err != nil {
			ch <- err
			return
		}

		if !skipFirst {
			dimensions := errplane.Dimensions{"host": AgentConfig.Hostname}

			total := float64(cpu.Total() - prevCpu.Total())

			sys := float64(cpu.Sys-prevCpu.Sys) / total * 100
			user := float64(cpu.User-prevCpu.User) / total * 100
			idle := float64(cpu.Idle-prevCpu.Idle) / total * 100
			wait := float64(cpu.Wait-prevCpu.Wait) / total * 100
			irq := float64(cpu.Irq-prevCpu.Irq) / total * 100
			softirq := float64(cpu.SoftIrq-prevCpu.SoftIrq) / total * 100
			stolen := float64(cpu.Stolen-prevCpu.Stolen) / total * 100

			if report(ep, "server.stats.cpu.sys", sys, timestamp, dimensions, ch) ||
				report(ep, "server.stats.cpu.user", user, timestamp, dimensions, ch) ||
				report(ep, "server.stats.cpu.idle", idle, timestamp, dimensions, ch) ||
				report(ep, "server.stats.cpu.wait", wait, timestamp, dimensions, ch) ||
				report(ep, "server.stats.cpu.irq", irq, timestamp, dimensions, ch) ||
				report(ep, "server.stats.cpu.softirq", softirq, timestamp, dimensions, ch) ||
				report(ep, "server.stats.cpu.stolen", stolen, timestamp, dimensions, ch) {
				return
			}
		}
		skipFirst = false
		prevCpu = cpu
		time.Sleep(AgentConfig.Sleep)
	}
}

func networkStats(ep *errplane.Errplane, ch chan error) {
	prevNetwork := NetworkUtilization{}
	for {
		network := NetworkUtilization{}
		err := network.Get()

		timestamp := time.Now()
		if err != nil {
			ch <- err
			return
		}

		for name, utilization := range network {
			if prevNetwork[name] == nil {
				continue
			}

			dimensions := errplane.Dimensions{"host": AgentConfig.Hostname, "device": name}

			rxBytes := float64(utilization.rxBytes - prevNetwork[name].rxBytes)
			rxPackets := float64(utilization.rxPackets - prevNetwork[name].rxPackets)
			rxDroppedPackets := float64(utilization.rxDroppedPackets - prevNetwork[name].rxDroppedPackets)
			rxErrors := float64(utilization.rxErrors - prevNetwork[name].rxErrors)

			txBytes := float64(utilization.txBytes - prevNetwork[name].txBytes)
			txPackets := float64(utilization.txPackets - prevNetwork[name].txPackets)
			txDroppedPackets := float64(utilization.txDroppedPackets - prevNetwork[name].txDroppedPackets)
			txErrors := float64(utilization.txErrors - prevNetwork[name].txErrors)

			if report(ep, "server.stats.network.rxBytes", rxBytes, timestamp, dimensions, ch) ||
				report(ep, "server.stats.network.rxPackets", rxPackets, timestamp, dimensions, ch) ||
				report(ep, "server.stats.network.rxDropped", rxDroppedPackets, timestamp, dimensions, ch) ||
				report(ep, "server.stats.network.rxErrors", rxErrors, timestamp, dimensions, ch) ||
				report(ep, "server.stats.network.txBytes", txBytes, timestamp, dimensions, ch) ||
				report(ep, "server.stats.network.txPackets", txPackets, timestamp, dimensions, ch) ||
				report(ep, "server.stats.network.txDropped", txDroppedPackets, timestamp, dimensions, ch) ||
				report(ep, "server.stats.network.txErrors", txErrors, timestamp, dimensions, ch) {
				return
			}
		}
		prevNetwork = network
		time.Sleep(AgentConfig.Sleep)
	}
}
