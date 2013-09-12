package main

import (
	log "code.google.com/p/log4go"
	"datastore"
	"flag"
	"fmt"
	"github.com/errplane/errplane-go"
	"github.com/errplane/errplane-go-common/agent"
	"github.com/errplane/gosigar"
	"io/ioutil"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"utils"
)

func main() {
	configFile := flag.String("config", "/etc/anomalous-agent/config.yml", "The agent config file")
	flag.Parse()

	pidFile := flag.String("pidfile", "/data/anomalous-agent/shared/anomalous-agent.pid", "The agent pid file")
	flag.Parse()

	config, err := utils.ParseConfig(*configFile)
	if err != nil {
		fmt.Printf("Error while reading configuration. Error: %s", err)
		os.Exit(1)
	}

	if *pidFile == "" {
		fmt.Printf("Pidfile is a required argument and cannot be empty")
		os.Exit(1)
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
	if err := agent.start(); err != nil {
		log.Error("Error occured while running the agent. Error: %s", err)
		os.Exit(1)
	}
	os.Exit(0)
}

type Reporter interface {
	Report(metric string, value float64, timestamp time.Time, context string, dimensions errplane.Dimensions)
}

type Agent struct {
	config              *utils.Config
	configClient        *utils.ConfigServiceClient
	timeseriesDatastore *datastore.TimeseriesDatastore
	snapshotDatastore   *datastore.SnapshotDatastore
	ep                  *errplane.Errplane
	detector            *AnomaliesDetector
	websocketClient     *WebsocketClient
}

func NewAgent(config *utils.Config) (*Agent, error) {
	ep := errplane.New(config.AppKey, config.Environment, config.ApiKey)
	ep.SetHttpHost(config.HttpHost)
	ep.SetUdpAddr(config.UdpHost)
	if config.Proxy != "" {
		ep.SetProxy(config.Proxy)
	}

	configClient := utils.NewConfigServiceClient(config)

	timeseriesDatastore, err := datastore.NewTimeseriesDatastore(config.DatastoreDir)
	if err != nil {
		return nil, err
	}

	snapshotDatastore, err := datastore.NewSnapshotDatastore(config.DatastoreDir, config.Database(), timeseriesDatastore)
	if err != nil {
		return nil, err
	}

	agent := &Agent{
		config:              config,
		configClient:        configClient,
		timeseriesDatastore: timeseriesDatastore,
		snapshotDatastore:   snapshotDatastore,
		ep:                  ep,
	}

	return agent, nil
}

func (self *Agent) start() error {
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
	//go self.startUdpListener()
	go self.startLocalServer()
	self.detector = NewAnomaliesDetector(self.config, self.configClient, self, self.timeseriesDatastore)
	self.detector.Start()
	self.websocketClient = NewWebsocketClient(self.config, self.detector, self.timeseriesDatastore, self.snapshotDatastore)
	self.websocketClient.Start()
	go self.watchLogFile()
	log.Info("Agent started successfully")

	// TODO: handle a shutdown
	for {
		err = <-ch
		log.Error(err)
	}
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
	log.Debug("Reporting %s", metric)

	self.detector.Report(metric, value, context, dimensions)

	time := timestamp.Unix()

	protobufDimensions := make([]*agent.Dimension, 0, len(dimensions))
	for name, value := range dimensions {
		localName := name
		localValue := value
		protobufDimensions = append(protobufDimensions, &agent.Dimension{
			Name:  &localName,
			Value: &localValue,
		})
	}

	self.timeseriesDatastore.WritePoints(self.config.Database(), metric, []*agent.Point{
		&agent.Point{
			Value:      &value,
			Time:       &time,
			Context:    &context,
			Dimensions: protobufDimensions,
		},
	})
	if metric != "errplane.anomalies" {
		return
	}
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

	processName := ""
	var dimensions errplane.Dimensions

	// is this a monitored process or one of the top N processes ?
	if monitoredProcess != nil {
		processName = monitoredProcess.Nickname
	} else {
		processName = strconv.Itoa(stat.pid)
		dimensions = errplane.Dimensions{
			"name":    stat.name,
			"cmdline": strings.Join(stat.args, " "),
		}
	}

	switch metricName {
	case "cpu":
		metric = fmt.Sprintf("%s.processes.%s.cpu%s", self.config.Hostname, processName, suffix)
		value = stat.cpuUsage
	case "mem":
		metric = fmt.Sprintf("%s.processes.%s.mem%s", self.config.Hostname, processName, suffix)
		value = stat.memUsage
	default:
		log.Error("unknown metric name %s", metricName)
		return true
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
				isPartition, err := regexp.MatchString(".*[0-9]$", diskUsage.Name)
				if err != nil {
					log.Error("Error matching regex. Error: %s", err)
					isPartition = false
				}
				if strings.Contains(diskUsage.Name, "ram") || strings.Contains(diskUsage.Name, "loop") || isPartition {
					// ignore these device names
					continue
				}

				prevDiskUsage := devNameToDiskUsage[diskUsage.Name]
				if prevDiskUsage == nil {
					log.Warn("Cannot find %s in previous disk usage", diskUsage.Name)
					continue
				}

				millisecondsElapsed := timestamp.Sub(prevTimeStamp).Nanoseconds() / int64(time.Millisecond)
				utilization := float64(diskUsage.TotalIOTime-prevDiskUsage.TotalIOTime) / float64(millisecondsElapsed) * 100

				metric := fmt.Sprintf("%s.stats.io.%s", self.config.Hostname, diskUsage.Name)
				self.Report(metric, float64(utilization), timestamp, "", nil)
			}
		}

		prevDiskUsages = diskUsages
		prevTimeStamp = timestamp
		time.Sleep(self.config.Sleep)
	}
}

func (self *Agent) getServerStatMetricName(metricName string) string {
	return fmt.Sprintf("%s.stats.%s", self.config.Hostname, metricName)
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

		timestamp := time.Now()

		used := float64(mem.Used)
		actualUsed := float64(mem.ActualUsed)
		usedPercentage := actualUsed / float64(mem.Total) * 100

		if swap.Total > 0 {
			// report swap usage only if the server has swap enabled

			swapUsed := float64(swap.Used)
			swapUsedPercentage := swapUsed / float64(swap.Total) * 100

			self.Report(self.getServerStatMetricName("swap.used"), swapUsed, timestamp, "", nil)
			self.Report(self.getServerStatMetricName("swap.used_percentage"), swapUsedPercentage, timestamp, "", nil)
		}

		self.Report(self.getServerStatMetricName("memory.free"), float64(mem.Free), timestamp, "", nil)
		self.Report(self.getServerStatMetricName("memory.used"), used, timestamp, "", nil)
		self.Report(self.getServerStatMetricName("memory.actual_used"), actualUsed, timestamp, "", nil)
		self.Report(self.getServerStatMetricName("memory.used_percentage"), usedPercentage, timestamp, "", nil)
		self.Report(self.getServerStatMetricName("swap.free"), float64(swap.Free), timestamp, "", nil)

		time.Sleep(self.config.Sleep)
	}
}

func (self *Agent) diskSpaceStats(ch chan error) {
	fslist := sigar.FileSystemList{}

	for {
		fslist.Get()

		timestamp := time.Now()

		for _, fs := range fslist.List {
			if strings.HasPrefix(fs.DirName, "/sys") || strings.HasPrefix(fs.DirName, "/run") ||
				strings.HasPrefix(fs.DirName, "/dev") {
				// ignore these special directories
				continue
			}

			dir_name := fs.DirName

			usage := sigar.FileSystemUsage{}
			usage.Get(dir_name)

			used := float64(usage.Total)
			usedPercentage := usage.UsePercent()

			metricName := self.getServerStatMetricName(fmt.Sprintf("disk.%s.used", fs.DirName))
			self.Report(metricName, used, timestamp, "", nil)
			self.Report(metricName+"_percentage", usedPercentage, timestamp, "", nil)
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
			total := float64(cpu.Total() - prevCpu.Total())

			sys := float64(cpu.Sys-prevCpu.Sys) / total * 100
			user := float64(cpu.User-prevCpu.User) / total * 100
			idle := float64(cpu.Idle-prevCpu.Idle) / total * 100
			wait := float64(cpu.Wait-prevCpu.Wait) / total * 100
			irq := float64(cpu.Irq-prevCpu.Irq) / total * 100
			softirq := float64(cpu.SoftIrq-prevCpu.SoftIrq) / total * 100
			stolen := float64(cpu.Stolen-prevCpu.Stolen) / total * 100

			self.Report(self.getServerStatMetricName("cpu.sys"), sys, timestamp, "", nil)
			self.Report(self.getServerStatMetricName("cpu.user"), user, timestamp, "", nil)
			self.Report(self.getServerStatMetricName("cpu.idle"), idle, timestamp, "", nil)
			self.Report(self.getServerStatMetricName("cpu.wait"), wait, timestamp, "", nil)
			self.Report(self.getServerStatMetricName("cpu.irq"), irq, timestamp, "", nil)
			self.Report(self.getServerStatMetricName("cpu.softirq"), softirq, timestamp, "", nil)
			self.Report(self.getServerStatMetricName("cpu.stolen"), stolen, timestamp, "", nil)
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
				rxBytes := float64(utilization.rxBytes - prevNetwork[name].rxBytes)
				rxPackets := float64(utilization.rxPackets - prevNetwork[name].rxPackets)
				rxDroppedPackets := float64(utilization.rxDroppedPackets - prevNetwork[name].rxDroppedPackets)
				rxErrors := float64(utilization.rxErrors - prevNetwork[name].rxErrors)

				txBytes := float64(utilization.txBytes - prevNetwork[name].txBytes)
				txPackets := float64(utilization.txPackets - prevNetwork[name].txPackets)
				txDroppedPackets := float64(utilization.txDroppedPackets - prevNetwork[name].txDroppedPackets)
				txErrors := float64(utilization.txErrors - prevNetwork[name].txErrors)

				metricPrefix := self.getServerStatMetricName(fmt.Sprintf("network.%s.", name))

				self.Report(metricPrefix+"rxBytes", rxBytes, timestamp, "", nil)
				self.Report(metricPrefix+"rxPackets", rxPackets, timestamp, "", nil)
				self.Report(metricPrefix+"rxDropped", rxDroppedPackets, timestamp, "", nil)
				self.Report(metricPrefix+"rxErrors", rxErrors, timestamp, "", nil)
				self.Report(metricPrefix+"txBytes", txBytes, timestamp, "", nil)
				self.Report(metricPrefix+"txPackets", txPackets, timestamp, "", nil)
				self.Report(metricPrefix+"txDropped", txDroppedPackets, timestamp, "", nil)
				self.Report(metricPrefix+"txErrors", txErrors, timestamp, "", nil)
			}
		}
		skipFirst = false
		prevNetwork = network
		time.Sleep(self.config.Sleep)
	}
}
