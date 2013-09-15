package main

import (
	"bytes"
	"datastore"
	"fmt"
	"github.com/errplane/errplane-go"
	"github.com/errplane/errplane-go-common/agent"
	"github.com/errplane/errplane-go-common/monitoring"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path"
	"time"
	"utils"
)

type LogMonitoringSuite struct {
	reporter *ReporterMock
	agent    *Agent
	tempFile string
	dbDir    string
}

var _ = Suite(&LogMonitoringSuite{})

/* Mocks */

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

func (self *ReporterMock) Report(metric string, value float64, timestamp time.Time, context string, dimensions errplane.Dimensions) {
	self.events = append(self.events, &MockedEvent{metric, value, timestamp, context, dimensions})
}

func (self *ReporterMock) TakeSnapshot(snapshotRequests []*datastore.SnapshotRequest) (*agent.Snapshot, error) {
	return &agent.Snapshot{}, nil
}

func (self *LogMonitoringSuite) SetUpSuite(c *C) {
	self.reporter = &ReporterMock{}
	self.dbDir = path.Join(os.TempDir(), "db")
	config := &utils.Config{
		Sleep:        1 * time.Second,
		DatastoreDir: self.dbDir,
	}
	var err error
	self.agent, err = NewAgent(config)
	c.Assert(err, IsNil)
	configClient := utils.NewConfigServiceClient(config)
	dbDir, _ := ioutil.TempDir(os.TempDir(), "db")
	db, _ := datastore.NewTimeseriesDatastore(dbDir)
	self.agent.detector = NewAnomaliesDetector(config, configClient, self.reporter, db)
	self.agent.detector.Start()
	ioutil.WriteFile("/tmp/foo.txt", nil, 0644)
	go self.agent.watchLogFile()
}

func (self *LogMonitoringSuite) TearDownSuite(c *C) {
	os.RemoveAll(self.dbDir)
}

func (self *LogMonitoringSuite) SetUpTest(c *C) {
	tempFile, err := ioutil.TempFile(os.TempDir(), "logfile")
	self.tempFile = tempFile.Name()
	c.Assert(err, IsNil)

	// create monitoring config
	config := &monitoring.MonitorConfig{
		Monitors: []*monitoring.Monitor{
			&monitoring.Monitor{
				LogName: self.tempFile,
				Conditions: []*monitoring.Condition{
					&monitoring.Condition{
						AlertWhen:      monitoring.GREATER_THAN,
						AlertThreshold: 2,
						AlertOnMatch:   ".*WARN.*",
						OnlyAfter:      2 * time.Second,
					},
					&monitoring.Condition{
						AlertWhen:      monitoring.GREATER_THAN,
						AlertThreshold: 1,
						AlertOnMatch:   ".*ERROR.*",
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
			&monitoring.Monitor{
				PluginName: "redis",
				Conditions: []*monitoring.Condition{
					&monitoring.Condition{
						AlertOnMatch: "critical",
						OnlyAfter:    2 * time.Second,
					},
				},
			},
		},
	}
	self.agent.detector.monitoringConfig = config
	self.reporter.events = nil
}

func (self *LogMonitoringSuite) TestLogStorage(c *C) {
	time.Sleep(1 * time.Second)

	file, err := os.OpenFile(self.tempFile, os.O_APPEND|os.O_RDWR, 0644)
	c.Assert(err, IsNil)
	content := "INFO: testing\n"
	fmt.Fprint(file, content)
	file.Close()

	time.Sleep(1 * time.Second)

	points := []*agent.Point{}
	params := &datastore.GetParams{
		TimeSeries: fmt.Sprintf(".logs.%s", self.tempFile),
		StartTime:  time.Now().Add(-1 * time.Hour).Unix(),
	}
	self.agent.timeseriesDatastore.ReadSeries(params, func(p *agent.Point) error {
		points = append(points, p)
		return nil
	})
	c.Assert(points, HasLen, 1)
	c.Assert(*points[0].Context, Equals, content)
}

func (self *LogMonitoringSuite) TestLogMonitoring(c *C) {
	time.Sleep(1 * time.Second)

	file, err := os.OpenFile(self.tempFile, os.O_APPEND|os.O_RDWR, 0644)
	c.Assert(err, IsNil)
	fmt.Fprint(file, "WARN: testing\n")
	fmt.Fprint(file, "WARN: testing should exist\n")
	file.Close()

	time.Sleep(1 * time.Second)

	c.Assert(self.reporter.events, HasLen, 1)
	c.Assert(self.reporter.events[0].value, Equals, 2.0)
	c.Assert(self.reporter.events[0].context, Equals, "")
	c.Assert(self.reporter.events[0].dimensions, DeepEquals, errplane.Dimensions{
		"logFile":        self.tempFile,
		"alertWhen":      monitoring.GREATER_THAN.String(),
		"alertThreshold": "2",
		"alertOnMatch":   ".*WARN.*",
		"onlyAfter":      "2s",
	})
}

func (self *LogMonitoringSuite) TestLogContext(c *C) {
	time.Sleep(1 * time.Second)

	file, err := os.OpenFile(self.tempFile, os.O_APPEND|os.O_RDWR, 0644)
	c.Assert(err, IsNil)
	buffer := bytes.NewBufferString("")
	for i := 0; i < 10; i++ {
		buffer.WriteString("INFO: some info\n")
		fmt.Fprint(file, "INFO: some info\n")
	}
	fmt.Fprint(file, "ERROR: testing should exist\n")
	buffer.WriteString("ERROR: testing should exist\n")
	for i := 0; i < 10; i++ {
		if i < 9 {
			buffer.WriteString("INFO: some info\n")
		}
		fmt.Fprint(file, "INFO: some info\n")
	}
	buffer.WriteString("INFO: some info")
	file.Close()

	time.Sleep(1 * time.Second)

	c.Assert(self.reporter.events, HasLen, 1)
	c.Assert(self.reporter.events[0].value, Equals, 1.0)
	c.Assert(self.reporter.events[0].context, Equals, buffer.String())
	c.Assert(self.reporter.events[0].dimensions, DeepEquals, errplane.Dimensions{
		"logFile":        self.tempFile,
		"alertWhen":      monitoring.GREATER_THAN.String(),
		"alertThreshold": "1",
		"alertOnMatch":   ".*ERROR.*",
		"onlyAfter":      "2s",
	})
}

func (self *LogMonitoringSuite) TestResetMetricMonitoring(c *C) {
	// test resetting of the metric monitoring if the value of the metric
	// went below the threshold, i.e. cpu went below the threshold
	self.agent.detector.Report("foo.bar", 95.0, "", nil)

	time.Sleep(1 * time.Second)

	self.agent.detector.Report("foo.bar", 85.0, "", nil)

	c.Assert(self.reporter.events, HasLen, 0)
}

func (self *LogMonitoringSuite) TestMetricMonitoring(c *C) {
	self.agent.detector.Report("foo.bar", 95.0, "", nil)

	time.Sleep(2 * time.Second)

	self.agent.detector.Report("foo.bar", 95.0, "", nil)

	c.Assert(self.reporter.events, HasLen, 1)
	c.Assert(self.reporter.events[0].value, Equals, 1.0)
	c.Assert(self.reporter.events[0].dimensions["statName"], Equals, "foo.bar")
	c.Assert(self.reporter.events[0].dimensions["alertWhen"], Equals, ">")
	c.Assert(self.reporter.events[0].dimensions["alertThreshold"], Equals, "90")
	c.Assert(self.reporter.events[0].dimensions["onlyAfter"], Equals, "2s")
}

func (self *LogMonitoringSuite) TestPluginMonitoring(c *C) {
	self.agent.detector.Report("localhost.plugins.redis.status", 1.0, "critical", nil)

	time.Sleep(2 * time.Second)

	self.agent.detector.Report("localhost.plugins.redis.status", 1.0, "critical", nil)

	c.Assert(self.reporter.events, HasLen, 1)
	c.Assert(self.reporter.events[0].value, Equals, 1.0)
	c.Assert(self.reporter.events[0].dimensions["pluginName"], Equals, "redis")
	c.Assert(self.reporter.events[0].dimensions["alertOnMatch"], Equals, "critical")
	c.Assert(self.reporter.events[0].dimensions["onlyAfter"], Equals, "2s")
}

func (self *LogMonitoringSuite) TestResetPluginMonitoring(c *C) {
	self.agent.detector.Report("plugins.redis.status", 1.0, "critical", nil)

	time.Sleep(2 * time.Second)

	self.agent.detector.Report("plugins.redis.status", 1.0, "warning", nil)

	c.Assert(self.reporter.events, HasLen, 0)
}
