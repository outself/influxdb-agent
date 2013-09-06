package datastore

import (
	. "github.com/errplane/errplane-go-common/agent"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"time"
)

type TimeseriesDatastoreSuite struct {
	dbDir string
}

var _ = Suite(&TimeseriesDatastoreSuite{})

func (self *TimeseriesDatastoreSuite) SetUpTest(c *C) {
	var err error
	self.dbDir, err = ioutil.TempDir(os.TempDir(), "db")
	c.Assert(err, IsNil)
	err = os.MkdirAll(self.dbDir, 0644)
	c.Assert(err, IsNil)
}

func (self *TimeseriesDatastoreSuite) TearDownTest(c *C) {
	if self.dbDir != "" {
		err := os.RemoveAll(self.dbDir)
		c.Assert(err, IsNil)
	}
}

func (self *TimeseriesDatastoreSuite) TestOneDay(c *C) {
	db, err := NewTimeseriesDatastore(self.dbDir)
	c.Assert(err, IsNil)

	timestamp1 := time.Now().Add(-5 * time.Second).Unix()
	value1 := 1.0
	timestamp2 := time.Now().Unix()
	value2 := 2.0
	var sequence uint32 = 1

	err = db.WritePoints("dbname", "timeseries", []*Point{
		&Point{
			Time:           &timestamp1,
			Value:          &value1,
			SequenceNumber: &sequence,
		},
		&Point{
			Time:           &timestamp2,
			Value:          &value2,
			SequenceNumber: &sequence,
		},
	})

	c.Assert(err, IsNil)

	points := make([]*Point, 0)
	err = db.ReadSeries(&GetParams{
		database:   "dbname",
		timeSeries: "timeseries",
		startTime:  timestamp1,
		endTime:    timestamp2,
	}, func(p *Point) error {
		points = append(points, p)
		return nil
	})

	c.Assert(err, IsNil)
	c.Assert(points, HasLen, 2)
	c.Assert(*points[0].Value, Equals, 2.0)
	c.Assert(*points[1].Value, Equals, 1.0)
}

func (self *TimeseriesDatastoreSuite) TestMultipleDays(c *C) {}
