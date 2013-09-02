package main

import (
	"fmt"
	"github.com/errplane/errplane-go"
	"github.com/errplane/errplane-go-common/monitoring"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"time"
	. "utils"
)

type LogMonitoringSuite struct {
	reporter *ReporterMock
	detector *AnomaliesDetector
}

var _ = Suite(&LogMonitoringSuite{})

/* Mocks */

type MetricEvent struct {
	metric string
	value  float64
}

type LogEvent struct {
	filename string
	oldLines []string
	newLines []string
}

type MockedEvent struct {
	metric     string
	value      float64
	timestamp  time.Time
	context    string
	dimensions errplane.Dimensions
}

type ReporterMock struct {
	events []*MockedEvent
}

func (self *ReporterMock) Report(metric string, value float64, timestamp time.Time, context string, dimensions errplane.Dimensions) error {
	self.events = append(self.events, &MockedEvent{metric, value, timestamp, context, dimensions})
	return nil
}

func (self *LogMonitoringSuite) SetUpSuite(c *C) {
	// create monitoring config
	config := &monitoring.MonitorConfig{
		Monitors: []*monitoring.Monitor{
			&monitoring.Monitor{
				LogName: "/tmp/foo.txt",
				Conditions: []*monitoring.Condition{
					&monitoring.Condition{
						AlertWhen:      monitoring.GREATER_THAN,
						AlertThreshold: 1,
						AlertOnMatch:   ".*WARN.*",
						OnlyAfter:      2 * time.Second,
					},
				},
			},
			&monitoring.Monitor{
				StatName: "foo.bar",
				Conditions: []*monitoring.Condition{
					&monitoring.Condition{
						AlertWhen:      monitoring.GREATER_THAN,
						AlertThreshold: 90.0,
						OnlyAfter:      2 * time.Second,
					},
				},
			},
		},
	}

	self.reporter = &ReporterMock{}
	self.detector = NewAnomaliesDetector(self.reporter)
	ioutil.WriteFile("/tmp/foo.txt", nil, 0644)
	self.detector.config = config
	AgentConfig.Sleep = 1 * time.Second
	go watchLogFile(self.detector)
}

func (self *LogMonitoringSuite) SetUpTest(c *C) {
	self.reporter.events = nil
}

func (self *LogMonitoringSuite) TestLogMonitoring(c *C) {
	time.Sleep(1 * time.Second)

	file, err := os.OpenFile("/tmp/foo.txt", os.O_APPEND|os.O_RDWR, 0644)
	c.Assert(err, IsNil)
	fmt.Fprint(file, "WARN: testing\n")
	fmt.Fprint(file, "WARN: testing should exist\n")
	file.Close()

	time.Sleep(1 * time.Second)

	c.Assert(self.reporter.events, HasLen, 1)
	c.Assert(self.reporter.events[0].value, Equals, 2.0)
	c.Assert(self.reporter.events[0].dimensions, DeepEquals, errplane.Dimensions{
		"LogFile":        "/tmp/foo.txt",
		"AlertWhen":      monitoring.GREATER_THAN.String(),
		"AlertThreshold": "1",
		"AlertOnMatch":   ".*WARN.*",
		"OnlyAfter":      "2s",
	})
}

func (self *LogMonitoringSuite) TestResetMetricMonitoring(c *C) {
	// test resetting of the metric monitoring if the value of the metric
	// went below the threshold, i.e. cpu went below the threshold
	self.detector.ReportMetricEvent("foo.bar", 95.0)

	time.Sleep(1 * time.Second)

	self.detector.ReportMetricEvent("foo.bar", 85.0)

	c.Assert(self.reporter.events, HasLen, 0)
}

func (self *LogMonitoringSuite) TestMetricMonitoring(c *C) {
	self.detector.ReportMetricEvent("foo.bar", 95.0)

	time.Sleep(2 * time.Second)

	self.detector.ReportMetricEvent("foo.bar", 95.0)

	c.Assert(self.reporter.events, HasLen, 1)
	c.Assert(self.reporter.events[0].value, Equals, 1.0)
	c.Assert(self.reporter.events[0].dimensions["StatName"], Equals, "foo.bar")
	c.Assert(self.reporter.events[0].dimensions["AlertWhen"], Equals, ">")
	c.Assert(self.reporter.events[0].dimensions["AlertThreshold"], Equals, "90")
	c.Assert(self.reporter.events[0].dimensions["OnlyAfter"], Equals, "2s")
}
