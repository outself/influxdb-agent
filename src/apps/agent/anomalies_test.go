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

type LogMonitoringSuite struct{}

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

func (self *LogMonitoringSuite) TestParsing(c *C) {
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
						OnlyAfter:      4 * time.Second,
					},
				},
			},
		},
	}

	ioutil.WriteFile("/tmp/foo.txt", nil, 0644)

	reporter := &ReporterMock{}
	detector := NewAnomaliesDetector(reporter)
	detector.config = config
	AgentConfig.Sleep = 1 * time.Second
	go watchLogFile(detector)

	time.Sleep(2 * time.Second)

	file, err := os.OpenFile("/tmp/foo.txt", os.O_APPEND|os.O_RDWR, 0644)
	c.Assert(err, IsNil)
	fmt.Fprint(file, "WARN: testing\n")
	fmt.Fprint(file, "WARN: testing should exist\n")
	file.Close()

	time.Sleep(3 * time.Second)

	c.Assert(reporter.events, HasLen, 1)
	c.Assert(reporter.events[0].value, Equals, 2.0)
	c.Assert(reporter.events[0].dimensions, DeepEquals, errplane.Dimensions{
		"LogFile":        "/tmp/foo.txt",
		"AlertWhen":      monitoring.GREATER_THAN.String(),
		"AlertThreshold": "1",
		"AlertOnMatch":   ".*WARN.*",
		"OnlyAfter":      "4s",
	})
}
