package main

import (
	log "code.google.com/p/log4go"
	"crypto/md5"
	"datastore"
	"encoding/json"
	"fmt"
	"github.com/errplane/errplane-go"
	"github.com/errplane/errplane-go-common/agent"
	"github.com/errplane/errplane-go-common/monitoring"
	"github.com/pmylund/go-cache"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"utils"
)

type LogEvent struct {
	timestamp time.Time
	before    []string
	lines     string
	after     []string
}

type Event struct {
	timestamp time.Time
}

type ProcessEvent struct {
	status    utils.Status
	timestamp time.Time
}

type LogEvents struct {
	events []*LogEvent
}

type MetricEvents struct {
	events []*Event
}

type ProcessEvents struct {
	events []*ProcessEvent
}

var eventCache *cache.Cache

func init() {
	eventCache = cache.New(0, 0)
}

type Detector interface {
	filesToMonitor() []string
	ReportLogEvent(string, []string, []string)
	ReportProcessEvent(*agent.ProcessMonitor, utils.Status)
	Report(string, float64, string, errplane.Dimensions)
}

type AnomaliesDetector struct {
	monitoringConfig         *monitoring.MonitorConfig
	agentConfig              *utils.Config
	configClient             *utils.ConfigServiceClient
	reporter                 Reporter
	forceMonitorConfigUpdate chan int
	forceLogConfigUpdate     chan bool
	timeSeriesDatastore      *datastore.TimeseriesDatastore
}

func NewAnomaliesDetector(agentConfig *utils.Config, configClient *utils.ConfigServiceClient, reporter Reporter, timeSeriesDatastore *datastore.TimeseriesDatastore) *AnomaliesDetector {
	detector := &AnomaliesDetector{nil, agentConfig, configClient, reporter, make(chan int), make(chan bool), timeSeriesDatastore}
	return detector
}

func (self *AnomaliesDetector) Start() {
	go self.updateMonitorConfig()
}

func (self *AnomaliesDetector) ForceMonitorConfigUpdate() {
	self.forceMonitorConfigUpdate <- 1
	self.forceLogConfigUpdate <- true
}

func (self *AnomaliesDetector) updateMonitorConfig() {
	t := time.NewTicker(self.agentConfig.Sleep)
	for {
		var err error
		config, err := self.configClient.GetMonitoringConfig()
		if err != nil {
			log.Error("Failed to get monitoring configuration. Error: %s", err)
		} else {
			self.monitoringConfig = config
		}
		// now sleep until either a force update is sent or we just poll to check again
		select {
		case <-t.C:
			// do nothing
		case <-self.forceMonitorConfigUpdate:
			log.Info("Forcing reload of configuration...")
		}
	}
}

func (self *AnomaliesDetector) filesToMonitor() []string {
	if self.monitoringConfig == nil {
		return nil
	}

	paths := make([]string, 0)
	for _, monitor := range self.monitoringConfig.Monitors {
		if monitor.LogName == "" {
			continue
		}
		paths = append(paths, monitor.LogName)
	}
	return paths
}

func (self *AnomaliesDetector) Report(metricName string, value float64, context string, dimensions errplane.Dimensions) {
	if self.monitoringConfig == nil {
		return
	}

	for _, monitor := range self.monitoringConfig.Monitors {
		match := monitor.StatName == metricName
		var err error
		if monitor.StatRegex != "" {
			match, err = regexp.MatchString(monitor.StatRegex, metricName)
			if err != nil {
				log.Error("Invalid regex %s. Error: %s", monitor.StatRegex, err)
			}
		}

		if match {
			self.reportMetricEvent(monitor, value)
			continue
		}

		if monitor.PluginName == "" {
			// not a plugin monitor
			continue
		}

		pluginRegexp, _ := regexp.Compile("plugins\\.([^.]*)\\.status")
		matches := pluginRegexp.FindStringSubmatch(metricName)
		if len(matches) != 2 {
			// something is wrong or the metric name isn't a plugin status
			continue
		}

		pluginName := matches[1]
		match, err = regexp.MatchString(monitor.PluginName, pluginName)
		if err != nil {
			log.Error("Invalid regex %s. Error: %s", monitor.PluginName, err)
			continue
		}
		if !match {
			// doesn't match the monitor regex
			continue
		}
		status := context
		self.reportPluginEvent(monitor, pluginName, status)
		// stop processing any further plugin monitor
		break
	}
}

func (self *AnomaliesDetector) reportPluginEvent(monitor *monitoring.Monitor, name string, status string) {
	// if the monitor is currently snoozed, then just return
	if time.Now().Before(monitor.SnoozeUntil()) {
		return
	}

	// we have a monitor that matches the given filename
	for _, condition := range monitor.Conditions {
		ok, err := regexp.MatchString(condition.AlertOnMatch, status)
		if err != nil {
			log.Error("Error while matching regex: %s. Error: %s", condition.AlertWhen)
			return
		}
		// split lines and see if any one of them matches
		key := fmt.Sprintf("%#v/%#v", monitor, condition)
		if !ok {
			eventCache.Delete(key)
			return
		}

		_metricEvents, ok := eventCache.Get(key)
		if !ok {
			metricEvents := &MetricEvents{}
			eventCache.Set(key, metricEvents, 0)
			_metricEvents = metricEvents
		}

		metricEvents := _metricEvents.(*MetricEvents)
		metricEvents.events = append(metricEvents.events, &Event{time.Now()})

		if len(metricEvents.events) > 0 && time.Now().Sub(metricEvents.events[0].timestamp) > condition.OnlyAfter {
			if !self.isSilenced(monitor, condition) {
				self.reporter.Report("errplane.anomalies", 1.0, time.Now(), "", errplane.Dimensions{
					"type":         "plugin",
					"pluginName":   name,
					"alertOnMatch": condition.AlertOnMatch,
					"onlyAfter":    condition.OnlyAfter.String(),
					"host":         self.agentConfig.Hostname,
					"status":       status,
				})
			}
		}

		// remove all events that are older than "OnlyAfter"
		thresholdTime := time.Now().Add(-condition.OnlyAfter)
		var newEvents []*Event
		for idx, event := range metricEvents.events {
			if event.timestamp.After(thresholdTime) {
				newEvents = metricEvents.events[idx:]
				break
			}
		}
		metricEvents.events = newEvents
	}
}

func (self *AnomaliesDetector) reportMetricEvent(monitor *monitoring.Monitor, value float64) {
	if time.Now().Before(monitor.SnoozeUntil()) || monitor.Disabled {
		return
	}

	// we have a monitor that matches the given filename
	for _, condition := range monitor.Conditions {
		// split lines and see if any one of them matches
		key := fmt.Sprintf("%#v/%#v", monitor, condition)
		if !condition.AlertWhen.CrossedThreshold(condition.AlertThreshold, value) {
			eventCache.Delete(key)
			return
		}

		_metricEvents, ok := eventCache.Get(key)
		if !ok {
			metricEvents := &MetricEvents{}
			eventCache.Set(key, metricEvents, 0)
			_metricEvents = metricEvents
		}

		metricEvents := _metricEvents.(*MetricEvents)
		metricEvents.events = append(metricEvents.events, &Event{time.Now()})

		if len(metricEvents.events) > 0 && time.Now().Sub(metricEvents.events[0].timestamp) > condition.OnlyAfter {
			if !self.isSilenced(monitor, condition) {
				startTime := time.Now().Add(-30 * time.Minute).Unix()
				snapshotRequests := []*datastore.SnapshotRequest{
					&datastore.SnapshotRequest{Regex: fmt.Sprintf("%s\\.stats.*", self.agentConfig.Hostname), StartTime: startTime},
				}
				snapshot, err := self.reporter.TakeSnapshot(snapshotRequests)
				if err != nil {
					log.Error("Cannot generate anomaly report. Error: %s\n", utils.WrapInErrplaneError(err))
				} else {
					self.reporter.Report("errplane.anomalies", 1.0, time.Now(), "", errplane.Dimensions{
						"statName":       monitor.StatName,
						"statRegex":      monitor.StatRegex,
						"type":           "stat",
						"alertWhen":      condition.AlertWhen.String(),
						"alertThreshold": strconv.FormatFloat(condition.AlertThreshold, 'f', -1, 64),
						"onlyAfter":      condition.OnlyAfter.String(),
						"host":           self.agentConfig.Hostname,
						"monitorId":      monitor.Id,
						"snapshotId":     snapshot.GetId(),
					})
				}
			}
		}

		// remove all events that are older than "OnlyAfter"
		thresholdTime := time.Now().Add(-condition.OnlyAfter)
		var newEvents []*Event
		for idx, event := range metricEvents.events {
			if event.timestamp.After(thresholdTime) {
				newEvents = metricEvents.events[idx:]
				break
			}
		}
		metricEvents.events = newEvents
	}
}

func (self *AnomaliesDetector) ReportProcessEvent(process *agent.ProcessMonitor, state utils.Status) {
	if time.Now().Before(process.SnoozeUntil()) {
		return
	}

	key := fmt.Sprintf("processes/%s", process.Nickname)
	_processEvents, ok := eventCache.Get(key)
	if !ok {
		processEvents := &ProcessEvents{}
		eventCache.Set(key, processEvents, 0)
		_processEvents = processEvents
	}

	processEvents := _processEvents.(*ProcessEvents)

	if len(processEvents.events) == 0 {
		processEvents.events = append(processEvents.events, &ProcessEvent{
			timestamp: time.Now(),
			status:    state,
		})
		return
	}

	oldState := processEvents.events[0].status
	newState := state
	processEvents.events[0] = &ProcessEvent{
		timestamp: time.Now(),
		status:    state,
	}

	if oldState != newState {
		if newState == utils.UP {
			self.reportProcessUp(process)
		} else {
			// holy shit, process down!
			self.reportProcessDown(process)
		}
	}

	if newState == utils.DOWN {
		startProcess(process)
	}
}

func (self *AnomaliesDetector) reportProcessDown(process *agent.ProcessMonitor) {
	log.Info("Process %s went down", process.Nickname)
	startTime := time.Now().Add(-30 * time.Minute).Unix()
	snapshotRequests := []*datastore.SnapshotRequest{
		&datastore.SnapshotRequest{Regex: fmt.Sprintf("%s\\.stats.*", self.agentConfig.Hostname), StartTime: startTime},
		&datastore.SnapshotRequest{Regex: fmt.Sprintf("%s\\.process\\.%s.*", process.Nickname), StartTime: startTime},
		&datastore.SnapshotRequest{Regex: fmt.Sprintf("%s\\.logs.*", self.agentConfig.Hostname), StartTime: 1, Limit: 500},
	}
	snapshot, err := self.reporter.TakeSnapshot(snapshotRequests)
	if err != nil {
		log.Error("Cannot generate anomaly report. Error: %s\n", utils.WrapInErrplaneError(err))
	}
	self.reportProcessEvent(process, snapshot, "down")
}

func (self *AnomaliesDetector) reportProcessUp(process *agent.ProcessMonitor) {
	log.Info("Process %s came back up reporting event", process.Nickname)
	self.reportProcessEvent(process, nil, "up")
}

func (self *AnomaliesDetector) reportProcessEvent(process *agent.ProcessMonitor, snapshot *agent.Snapshot, status string) {
	if snapshot != nil {
		self.reporter.Report("errplane.anomalies", 1.0, time.Now(), "", errplane.Dimensions{
			"type":       "process",
			"process":    process.Nickname,
			"snapshotId": snapshot.GetId(),
			"host":       self.agentConfig.Hostname,
			"status":     "down",
			"monitorId":  process.Id,
		})

	} else {
		self.reporter.Report("errplane.anomalies", 1.0, time.Now(), "", errplane.Dimensions{
			"type":      "process",
			"process":   process.Nickname,
			"host":      self.agentConfig.Hostname,
			"status":    "up",
			"monitorId": process.Id,
		})
	}
}

func (self *AnomaliesDetector) ReportLogEvent(filename string, oldLines []string, newLines []string) {
	if self.monitoringConfig == nil {
		return
	}

	for _, monitor := range self.monitoringConfig.Monitors {
		if monitor.LogName != filename {
			continue
		}

		// we have a monitor that matches the given filename
		for _, condition := range monitor.Conditions {
			// split lines and see if any one of them matches
			matchingLines := make([]int, 0)
			for idx, line := range newLines {
				lowerCased := strings.ToLower(line)
				lowerCasedCondition := strings.ToLower(condition.AlertOnMatch)
				// log.Debug("Checking whether %s matches %s", lowerCasedCondition, lowerCased)
				if ok, err := regexp.MatchString(lowerCasedCondition, lowerCased); err != nil || !ok {
					if err != nil {
						log.Error("Invalid regex %s", condition.AlertOnMatch)
					}
					continue
				}
				matchingLines = append(matchingLines, idx)
			}

			if len(matchingLines) == 0 {
				continue
			}

			key := fmt.Sprintf("%#v/%#v", monitor, condition)
			_logEvents, ok := eventCache.Get(key)
			if !ok {
				logEvents := &LogEvents{}
				eventCache.Set(key, logEvents, 0)
				_logEvents = logEvents
			}

			logEvents := _logEvents.(*LogEvents)
			if condition.OnlyAfter == 0 || condition.AlertThreshold <= 1 {
				// if the time isn't set or if we're alerting on every match, then clear the old events
				logEvents.events = nil
			}

			for _, idx := range matchingLines {
				// get the previous 10 lines to create the context
				oldLinesIdx := 10 - idx
				before := []string{}
				if oldLinesIdx > 0 && oldLinesIdx < len(oldLines) {
					before = append(before, oldLines[oldLinesIdx:]...)
				}
				before = append(before, newLines[:idx]...)
				// get the following 10 lines
				lastLine := int(math.Min(float64(idx+11), float64(len(newLines))))
				after := newLines[idx+1 : lastLine]

				logEvents.events = append(logEvents.events, &LogEvent{time.Now(), before, newLines[idx], after})
			}

			if condition.OnlyAfter > 0 {

				// remove all events that are older than "OnlyAfter"
				thresholdTime := time.Now().Add(-condition.OnlyAfter)
				var newEvents []*LogEvent
				for idx, event := range logEvents.events {
					if event.timestamp.After(thresholdTime) {
						newEvents = logEvents.events[idx:]
						break
					}
				}
				logEvents.events = newEvents
			}

			if len(logEvents.events) >= int(condition.AlertThreshold) {
				context := ""
				if condition.AlertThreshold == 1 {
					event := logEvents.events[0]
					context = strings.Join(event.before, "\n") + "\n" + event.lines + "\n" + strings.Join(event.after, "\n")
				} else {
					allMatchingLines := make([]string, 0)
					for _, e := range logEvents.events {
						allMatchingLines = append(allMatchingLines, e.lines)
					}
					context = strings.Join(allMatchingLines, "\n")
				}

				if self.isSilenced(monitor, condition) {
					log.Debug("Suppressing alert, condition is temporarily silenced")
					continue
				}

				snapshotRequests := []*datastore.SnapshotRequest{
					&datastore.SnapshotRequest{Regex: fmt.Sprintf("%s\\.logs.*", self.agentConfig.Hostname), StartTime: 1, Limit: 500},
				}
				snapshot, err := self.reporter.TakeSnapshot(snapshotRequests)
				if err != nil {
					log.Error("Cannot generate anomaly report. Error: %s\n", utils.WrapInErrplaneError(err))
				} else {
					self.reporter.Report("errplane.anomalies", float64(len(logEvents.events)), time.Now(), context, errplane.Dimensions{
						"logFile":        monitor.LogName,
						"type":           "log",
						"alertWhen":      condition.AlertWhen.String(),
						"alertThreshold": strconv.FormatFloat(condition.AlertThreshold, 'f', -1, 64),
						"alertOnMatch":   condition.AlertOnMatch,
						"onlyAfter":      condition.OnlyAfter.String(),
						"host":           self.agentConfig.Hostname,
						"monitorId":      monitor.Id,
						"snapshotId":     snapshot.GetId(),
					})
				}
			}
		}
	}
}

func (self *AnomaliesDetector) isSilenced(monitor *monitoring.Monitor, condition *monitoring.Condition) bool {
	// this is my ghetto way to hash these things into a key so I can look up when they were last alerted
	jsm, _ := json.Marshal(monitor)
	jsc, _ := json.Marshal(condition)
	key := string(jsm) + string(jsc)
	h := md5.New()
	io.WriteString(h, key)
	seriesName := fmt.Sprintf("%s", h.Sum(nil))

	// now check to make sure we haven't sent out too many alerts for each silence setting
	now := time.Now()
	endTime := now.Unix()
	database := self.agentConfig.Database()
	for _, setting := range self.monitoringConfig.Silence {
		startTime := now.Add(-setting.Duration).Unix()
		params := &datastore.GetParams{StartTime: startTime, EndTime: endTime, Limit: int64(setting.Count + 1), TimeSeries: seriesName, Database: database}
		count := 0
		cb := func(point *agent.Point) error {
			count += 1
			return nil
		}
		self.timeSeriesDatastore.ReadSeries(params, cb)
		if count >= setting.Count {
			return true
		}
	}

	// now that we know we haven't, we'll be sending out an alert, so mark this so we can avoid flooding with alerts
	points := make([]*agent.Point, 1, 1)
	points[0] = &agent.Point{Time: &endTime}
	self.timeSeriesDatastore.WritePoints(database, seriesName, points)
	return false
}
