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
	"utils"
)

func main() {
	configFile := flag.String("config", "/etc/errplane-agent/config.yml", "The agent config file")
	flag.Parse()

	pidFile := flag.String("pidfile", "/data/errplane-agent/shared/errplane-agent.pid", "The agent pid file")
	flag.Parse()

	config, err := utils.ParseConfig(*configFile)
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

	agent, err := NewAgent(config)
	if err != nil {
		fmt.Printf("Error while initializing the agent. Error: %s", err)
		os.Exit(1)
	}
	if err := agent.startAgent(); err != nil {
		log.Error("Error occured while running the agent. Error: %s", err)
		os.Exit(1)
	}
	os.Exit(0)
}

type Reporter interface {
	Report(metric string, value float64, timestamp time.Time, context string, dimensions errplane.Dimensions)
}

type Agent struct {
	config       *utils.Config
	configClient *utils.ConfigServiceClient
	ep           *errplane.Errplane
}

func NewAgent(config *utils.Config) (*Agent, error) {
	ep := errplane.New(config.AppKey, config.Environment, config.ApiKey)
	ep.SetHttpHost(config.HttpHost)
	ep.SetUdpAddr(config.UdpHost)
	if config.Proxy != "" {
		ep.SetProxy(config.Proxy)
	}

	configClient := utils.NewConfigServiceClient(config)

	agent := &Agent{
		config:       config,
		configClient: configClient,
		ep:           ep,
	}

	return agent, nil
}

func (self *Agent) startAgent() error {
	err := self.initLog()
	if err != nil {
		return utils.WrapInErrplaneError(err)
	}

	ch := make(chan error)
	go self.memStats(ch)
	go self.cpuStats(ch)
	go self.networkStats(ch)
	go self.diskSpaceStats(ch)
	go self.ioStats(ch)
	go self.procStats(ch)
	go self.monitorProceses(ch)
	go self.monitorPlugins()
	go self.checkNewPlugins()
	go self.startUdpListener()
	go self.startLocalServer()
	detector := NewAnomaliesDetector(self.config, self.configClient, self)
	go self.watchLogFile(detector)
	log.Info("Agent started successfully")
	err = <-ch
	time.Sleep(1 * time.Second) // give the logger a chance to close and write to the file
	return utils.WrapInErrplaneError(err)
}

func (self *Agent) initLog() error {
	level := log.DEBUG
	switch self.config.LogLevel {
	case "info":
		level = log.INFO
	case "warn":
		level = log.WARNING
	case "error":
		level = log.ERROR
	}

	log.AddFilter("file", level, log.NewFileLogWriter(self.config.LogFile, false))

	var err error
	os.Stderr, err = os.OpenFile(self.config.LogFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	os.Stdout = os.Stderr

	return nil
}

func (self *Agent) Report(metric string, value float64, timestamp time.Time, context string, dimensions errplane.Dimensions) {
	err := self.ep.Report(metric, value, timestamp, "", dimensions)
	if err != nil {
		log.Error("Error while sending report. Error: %s", err)
	}
}

func (self *Agent) procStats(ch chan error) {
	var previousStats map[int]*ProcStat

	for {
		_, procStats := getProcesses()

		if previousStats != nil {
			mergedStats := mergeStats(previousStats, procStats)

			n := int(math.Min(float64(self.config.TopNProcesses), float64(len(mergedStats))))

			sort.Sort(ProcStatsSortableByCpu(mergedStats))
			topNByCpu := mergedStats[0:n]
			now := time.Now()
			for _, stat := range topNByCpu {
				if self.reportProcessCpuUsage(nil, &stat, now, true, ch) {
					return
				}
			}
			sort.Sort(ProcStatsSortableByMem(mergedStats))
			topNByMem := mergedStats[0:n]
			for _, stat := range topNByMem {
				if self.reportProcessMemUsage(nil, &stat, now, true, ch) {
					return
				}
			}
		}

		previousStats = procStats
		time.Sleep(self.config.TopNSleep)
	}
}

func (self *Agent) reportProcessCpuUsage(monitoredProcess *utils.Process, stat *MergedProcStat, now time.Time, top bool, ch chan error) bool {
	return self.reportProcessMetric(monitoredProcess, stat, "cpu", now, top, ch)
}

func (self *Agent) reportProcessMemUsage(monitoredProcess *utils.Process, stat *MergedProcStat, now time.Time, top bool, ch chan error) bool {
	return self.reportProcessMetric(monitoredProcess, stat, "mem", now, top, ch)
}

func (self *Agent) reportProcessMetric(monitoredProcess *utils.Process, stat *MergedProcStat, metricName string, now time.Time, top bool, ch chan error) bool {
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
			"host":     self.config.Hostname,
		}
	} else {
		dimensions = errplane.Dimensions{
			"pid":     strconv.Itoa(stat.pid),
			"name":    stat.name,
			"cmdline": strings.Join(stat.args, " "),
			"host":    self.config.Hostname,
		}
	}

	self.Report(metric, value, now, "", dimensions)
	return false
}

func (self *Agent) ioStats(ch chan error) {
	prevTimeStamp := time.Now()
	var prevDiskUsages []utils.DiskUsage

	for {
		timestamp := time.Now()
		diskUsages, err := utils.GetDiskUsages()
		if err != nil {
			ch <- err
			return
		}

		if prevDiskUsages != nil {
			devNameToDiskUsage := make(map[string]*utils.DiskUsage)
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

				dimensions := errplane.Dimensions{"host": self.config.Hostname, "device": diskUsage.Name}

				self.Report("server.stats.io.utilization", float64(utilization), timestamp, "", dimensions)
			}
		}

		prevDiskUsages = diskUsages
		prevTimeStamp = timestamp
		time.Sleep(self.config.Sleep)
	}
}

func (self *Agent) memStats(ch chan error) {
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

		dimensions := errplane.Dimensions{"host": self.config.Hostname}
		timestamp := time.Now()

		used := float64(mem.Used)
		actualUsed := float64(mem.ActualUsed)
		usedPercentage := actualUsed / float64(mem.Total) * 100

		if swap.Total > 0 {
			// report swap usage only if the server has swap enabled

			swapUsed := float64(swap.Used)
			swapUsedPercentage := swapUsed / float64(swap.Total) * 100

			self.Report("server.stats.swap.used", swapUsed, timestamp, "", dimensions)
			self.Report("server.stats.swap.used_percentage", swapUsedPercentage, timestamp, "", dimensions)
		}

		self.Report("server.stats.memory.free", float64(mem.Free), timestamp, "", dimensions)
		self.Report("server.stats.memory.used", used, timestamp, "", dimensions)
		self.Report("server.stats.memory.actual_used", actualUsed, timestamp, "", dimensions)
		self.Report("server.stats.memory.used_percentage", usedPercentage, timestamp, "", dimensions)
		self.Report("server.stats.swap.free", float64(swap.Free), timestamp, "", dimensions)

		time.Sleep(self.config.Sleep)
	}
}

func (self *Agent) diskSpaceStats(ch chan error) {
	fslist := sigar.FileSystemList{}

	for {
		fslist.Get()

		timestamp := time.Now()

		for _, fs := range fslist.List {
			dir_name := fs.DirName

			usage := sigar.FileSystemUsage{}
			usage.Get(dir_name)

			dimensions := errplane.Dimensions{"host": self.config.Hostname, "device": fs.DevName}

			used := float64(usage.Total)
			usedPercentage := usage.UsePercent()

			self.Report("server.stats.disk.used", used, timestamp, "", dimensions)
			self.Report("server.stats.disk.used_percentage", usedPercentage, timestamp, "", dimensions)
		}
		time.Sleep(self.config.Sleep)
	}
}

func (self *Agent) cpuStats(ch chan error) {
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
			dimensions := errplane.Dimensions{"host": self.config.Hostname}

			total := float64(cpu.Total() - prevCpu.Total())

			sys := float64(cpu.Sys-prevCpu.Sys) / total * 100
			user := float64(cpu.User-prevCpu.User) / total * 100
			idle := float64(cpu.Idle-prevCpu.Idle) / total * 100
			wait := float64(cpu.Wait-prevCpu.Wait) / total * 100
			irq := float64(cpu.Irq-prevCpu.Irq) / total * 100
			softirq := float64(cpu.SoftIrq-prevCpu.SoftIrq) / total * 100
			stolen := float64(cpu.Stolen-prevCpu.Stolen) / total * 100

			self.Report("server.stats.cpu.sys", sys, timestamp, "", dimensions)
			self.Report("server.stats.cpu.user", user, timestamp, "", dimensions)
			self.Report("server.stats.cpu.idle", idle, timestamp, "", dimensions)
			self.Report("server.stats.cpu.wait", wait, timestamp, "", dimensions)
			self.Report("server.stats.cpu.irq", irq, timestamp, "", dimensions)
			self.Report("server.stats.cpu.softirq", softirq, timestamp, "", dimensions)
			self.Report("server.stats.cpu.stolen", stolen, timestamp, "", dimensions)
		}
		skipFirst = false
		prevCpu = cpu
		time.Sleep(self.config.Sleep)
	}
}

func (self *Agent) networkStats(ch chan error) {
	skipFirst := true

	prevNetwork := NetworkUtilization{}
	for {
		network := NetworkUtilization{}
		err := network.Get()

		timestamp := time.Now()
		if err != nil {
			ch <- err
			return
		}

		if !skipFirst {
			for name, utilization := range network {

				dimensions := errplane.Dimensions{"host": self.config.Hostname, "device": name}

				rxBytes := float64(utilization.rxBytes - prevNetwork[name].rxBytes)
				rxPackets := float64(utilization.rxPackets - prevNetwork[name].rxPackets)
				rxDroppedPackets := float64(utilization.rxDroppedPackets - prevNetwork[name].rxDroppedPackets)
				rxErrors := float64(utilization.rxErrors - prevNetwork[name].rxErrors)

				txBytes := float64(utilization.txBytes - prevNetwork[name].txBytes)
				txPackets := float64(utilization.txPackets - prevNetwork[name].txPackets)
				txDroppedPackets := float64(utilization.txDroppedPackets - prevNetwork[name].txDroppedPackets)
				txErrors := float64(utilization.txErrors - prevNetwork[name].txErrors)

				self.Report("server.stats.network.rxBytes", rxBytes, timestamp, "", dimensions)
				self.Report("server.stats.network.rxPackets", rxPackets, timestamp, "", dimensions)
				self.Report("server.stats.network.rxDropped", rxDroppedPackets, timestamp, "", dimensions)
				self.Report("server.stats.network.rxErrors", rxErrors, timestamp, "", dimensions)
				self.Report("server.stats.network.txBytes", txBytes, timestamp, "", dimensions)
				self.Report("server.stats.network.txPackets", txPackets, timestamp, "", dimensions)
				self.Report("server.stats.network.txDropped", txDroppedPackets, timestamp, "", dimensions)
				self.Report("server.stats.network.txErrors", txErrors, timestamp, "", dimensions)
			}
		}
		skipFirst = false
		prevNetwork = network
		time.Sleep(self.config.Sleep)
	}
}
