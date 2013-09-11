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

type LogEvents struct {
	events []*LogEvent
}

type MetricEvents struct {
	events []*Event
}

var eventCache *cache.Cache

func init() {
	eventCache = cache.New(0, 0)
}

type Detector interface {
	filesToMonitor() []string
	ReportLogEvent(string, []string, []string)
	Report(string, float64, string, errplane.Dimensions)
}

type AnomaliesDetector struct {
	monitoringConfig         *monitoring.MonitorConfig
	agentConfig              *utils.Config
	configClient             *utils.ConfigServiceClient
	reporter                 Reporter
	forceMonitorConfigUpdate chan int
	timeSeriesDatastore      *datastore.TimeseriesDatastore
}

func NewAnomaliesDetector(agentConfig *utils.Config, configClient *utils.ConfigServiceClient, reporter Reporter, timeSeriesDatastore *datastore.TimeseriesDatastore) *AnomaliesDetector {
	detector := &AnomaliesDetector{nil, agentConfig, configClient, reporter, make(chan int, 1), timeSeriesDatastore}
	return detector
}

func (self *AnomaliesDetector) Start() {
	go self.updateMonitorConfig()
}

func (self *AnomaliesDetector) ForceMonitorConfigUpdate() {
	self.forceMonitorConfigUpdate <- 1
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
		if monitor.StatName == metricName {
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
		match, err := regexp.MatchString(monitor.PluginName, pluginName)
		if !match || err != nil {
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
					"PluginName":   name,
					"AlertOnMatch": condition.AlertOnMatch,
					"OnlyAfter":    condition.OnlyAfter.String(),
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
	// we have a monitor that matches the given filename
	for _, condition := range monitor.Conditions {
		// split lines and see if any one of them matches
		key := fmt.Sprintf("%#v/%#v", monitor, condition)
		if value < condition.AlertThreshold {
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
					"StatName":       monitor.StatName,
					"AlertWhen":      condition.AlertWhen.String(),
					"AlertThreshold": strconv.FormatFloat(condition.AlertThreshold, 'f', -1, 64),
					"OnlyAfter":      condition.OnlyAfter.String(),
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

func (self *AnomaliesDetector) ReportLogEvent(filename string, oldLines []string, newLines []string) {
	// log.Debug("Inside ReportLogEvent")

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
				// log.Debug("%s matches %s", lowerCasedCondition, lowerCased)
				matchingLines = append(matchingLines, idx)
			}

			if len(matchingLines) == 0 {
				continue
			}

			// log.Debug("matches: %d", len(matchingLines))

			key := fmt.Sprintf("%#v/%#v", monitor, condition)
			_logEvents, ok := eventCache.Get(key)
			if !ok {
				logEvents := &LogEvents{}
				eventCache.Set(key, logEvents, 0)
				_logEvents = logEvents
			}

			logEvents := _logEvents.(*LogEvents)
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

			// remove all events that are older than "OnlyAfter"
			thresholdTime := time.Now().Add(-condition.OnlyAfter)
			var newEvents []*LogEvent
			// log.Debug("threshold time: %s", thresholdTime)
			for idx, event := range logEvents.events {
				// log.Debug("event timestamp: %s", event.timestamp)
				if event.timestamp.After(thresholdTime) {
					newEvents = logEvents.events[idx:]
					break
				}
			}
			logEvents.events = newEvents
			// log.Debug("new events: %d", len(logEvents.events))
			if len(logEvents.events) >= int(condition.AlertThreshold) {
				context := ""
				if condition.AlertThreshold == 1 {
					event := logEvents.events[0]
					context = strings.Join(event.before, "\n") + "\n" + event.lines + "\n" + strings.Join(event.after, "\n")
				}

				if !self.isSilenced(monitor, condition) {
					self.reporter.Report("errplane.anomalies", float64(len(logEvents.events)), time.Now(), context, errplane.Dimensions{
						"LogFile":        monitor.LogName,
						"AlertWhen":      condition.AlertWhen.String(),
						"AlertThreshold": strconv.FormatFloat(condition.AlertThreshold, 'f', -1, 64),
						"AlertOnMatch":   condition.AlertOnMatch,
						"OnlyAfter":      condition.OnlyAfter.String(),
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
	sequenceNumber := uint32(1)
	points[0] = &agent.Point{Time: &endTime, SequenceNumber: &sequenceNumber}
	self.timeSeriesDatastore.WritePoints(database, seriesName, points)
	return false
}
