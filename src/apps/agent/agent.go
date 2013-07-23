package main

import (
	log "code.google.com/p/log4go"
	"flag"
	"fmt"
	"github.com/cloudfoundry/gosigar"
	"github.com/errplane/errplane-go"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"time"
	"utils"
)

var (
	hostname    string
	proxy       string
	udpHost     string
	httpHost    string
	appKey      string
	environment string
	apiKey      string
	sleep       time.Duration
	logFile     string
	logLevel    string
)

func main() {
	config := flag.String("config", "/etc/errplane-agent/config.yml", "The agent config file")
	flag.Parse()

	err := initConfig(*config)
	if err != nil {
		fmt.Printf("Error while reading configuration. Error: %s", err)
		os.Exit(1)
	}

	err = initLog()
	if err != nil {
		fmt.Printf("Error while reading configuration. Error: %s", err)
		os.Exit(1)
	}

	ep := errplane.New(httpHost, udpHost, appKey, environment, apiKey)
	if proxy != "" {
		ep.SetProxy(proxy)
	}
	ch := make(chan error)
	go memStats(ep, ch)
	go cpuStats(ep, ch)
	go diskSpaceStats(ep, ch)
	go ioStats(ep, ch)
	log.Info("Agent started successfully")
	err = <-ch
	log.Error("Data collection stopped unexpectedly. Error: %s", err)
	log.Close()
	time.Sleep(1 * time.Second) // give the logger a chance to close and write to the file
	return
}

func initLog() error {
	level := log.DEBUG
	switch logLevel {
	case "info":
		level = log.INFO
	case "warn":
		level = log.WARNING
	case "error":
		level = log.ERROR
	}

	log.AddFilter("file", level, log.NewFileLogWriter(logFile, false))
	return nil
}

func initConfig(path string) error {
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		fmt.Printf("Cannot determine hostname. Error: %s\n", err)
		os.Exit(1)
	}

	content, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	m := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(content, &m)
	if err != nil {
		return err
	}
	general := m["general"].(map[interface{}]interface{})
	udpHost = general["udp-host"].(string)
	httpHost = general["http-host"].(string)
	environment = general["environment"].(string)
	appKey = general["app-key"].(string)
	apiKey = general["api-key"].(string)
	sleepStr := general["sleep"].(string)
	sleep, err = time.ParseDuration(sleepStr)
	if err != nil {
		return err
	}
	proxy_ := general["proxy"]
	if proxy_ != nil {
		proxy = proxy_.(string)
	}
	logFile = general["log-file"].(string)
	logLevel = general["log-level"].(string)
	return nil
}

func report(ep *errplane.Errplane, metric string, value float64, timestamp time.Time, dimensions errplane.Dimensions, ch chan error) bool {
	err := ep.Report(metric, value, timestamp, "", dimensions)
	if err != nil {
		ch <- err
		return true
	}
	return false
}

func ioStats(ep *errplane.Errplane, ch chan error) {
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

				dimensions := errplane.Dimensions{"host": hostname, "device": diskUsage.Name}

				if report(ep, "server.stats.io.utilization", float64(utilization), timestamp, dimensions, ch) {
					return
				}
			}
		}

		prevDiskUsages = diskUsages
		prevTimeStamp = timestamp
		time.Sleep(sleep)
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

		dimensions := errplane.Dimensions{"host": hostname}
		timestamp := time.Now()

		used := float64(mem.Used)
		actualUsed := float64(mem.ActualUsed)
		usedPercentage := actualUsed / float64(mem.Total) * 100

		swapUsed := float64(swap.Used)
		swapUsedPercentage := swapUsed / float64(swap.Total) * 100

		if report(ep, "server.stats.memory.free", float64(mem.Free), timestamp, dimensions, ch) ||
			report(ep, "server.stats.memory.used", used, timestamp, dimensions, ch) ||
			report(ep, "server.stats.memory.actual_used", actualUsed, timestamp, dimensions, ch) ||
			report(ep, "server.stats.memory.used_percentage", usedPercentage, timestamp, dimensions, ch) ||
			report(ep, "server.stats.swap.free", float64(swap.Free), timestamp, dimensions, ch) ||
			report(ep, "server.stats.swap.used", swapUsed, timestamp, dimensions, ch) ||
			report(ep, "server.stats.swap.used_percentage", swapUsedPercentage, timestamp, dimensions, ch) {
			return
		}

		time.Sleep(sleep)
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

			dimensions := errplane.Dimensions{"host": hostname, "device": fs.DevName}

			used := float64(usage.Total)
			usedPercentage := usage.UsePercent()

			if report(ep, "server.stats.disk.used", used, timestamp, dimensions, ch) ||
				report(ep, "server.stats.disk.used_percentage", usedPercentage, timestamp, dimensions, ch) {
				return
			}
		}
		time.Sleep(sleep)
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
			dimensions := errplane.Dimensions{"host": hostname}

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
		time.Sleep(sleep)
	}
}
