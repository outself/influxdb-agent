package datastore

import (
	protocol "github.com/errplane/errplane-go-common/agent"
	. "github.com/errplane/errplane-go-common/agent"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"time"
)

type DatastoreSuite struct {
	dbDir string
}

var _ = Suite(&DatastoreSuite{})

func (self *DatastoreSuite) SetUpTest(c *C) {
	var err error
	self.dbDir, err = ioutil.TempDir(os.TempDir(), "db")
	c.Assert(err, IsNil)
	err = os.MkdirAll(self.dbDir, 0644)
	c.Assert(err, IsNil)
}

func (self *DatastoreSuite) TearDownTest(c *C) {
	if self.dbDir != "" {
		err := os.RemoveAll(self.dbDir)
		c.Assert(err, IsNil)
	}
}

func (self *DatastoreSuite) testDataRetrievalCommon(c *C, timestamps ...int64) {
	db, err := NewTimeseriesDatastore(self.dbDir)
	defer db.Close()
	c.Assert(err, IsNil)

	var sequence uint32 = 1

	for idx, timestamp := range timestamps {
		value := float64(idx)
		err = db.WritePoints("dbname", "timeseries", []*Point{
			&Point{
				Time:           &timestamp,
				Value:          &value,
				SequenceNumber: &sequence,
			},
		})
	}

	c.Assert(err, IsNil)

	points := make([]*Point, 0)
	err = db.ReadSeries(&GetParams{
		database:   "dbname",
		timeSeries: "timeseries",
		startTime:  timestamps[0],
		endTime:    timestamps[len(timestamps)-1],
	}, func(p *Point) error {
		points = append(points, p)
		return nil
	})

	c.Assert(err, IsNil)
	c.Assert(points, HasLen, len(timestamps))
	for idx, _ := range timestamps {
		value := float64(len(timestamps) - idx - 1)
		c.Assert(*points[idx].Value, Equals, value)
	}

}

func (self *DatastoreSuite) TestOneDay(c *C) {
	timestamp1 := time.Now().Add(-5 * time.Second).Unix()
	timestamp2 := time.Now().Unix()
	self.testDataRetrievalCommon(c, timestamp1, timestamp2)
}

func (self *DatastoreSuite) TestMultipleDays(c *C) {
	timestamp1 := time.Now().Add(-48 * time.Hour).Unix()
	timestamp2 := time.Now().Add(-24 * time.Hour).Unix()
	self.testDataRetrievalCommon(c, timestamp1, timestamp2)
}

func (self *DatastoreSuite) TestMultipleDaysAndToday(c *C) {
	timestamp1 := time.Now().Add(-48 * time.Hour).Unix()
	timestamp2 := time.Now().Add(-24 * time.Hour).Unix()
	timestamp3 := time.Now().Unix()
	self.testDataRetrievalCommon(c, timestamp1, timestamp2, timestamp3)
}

func (self *DatastoreSuite) TestSnapshots(c *C) {
	db, err := NewSnapshotDatastore(self.dbDir)
	c.Assert(err, IsNil)
	c.Assert(db, NotNil)
	id := "snapshot1"
	t := time.Now().Unix()
	s := time.Now().Add(-5 * time.Minute).Unix()
	seriesName := "series1"
	snapshot := &protocol.Snapshot{
		Id:           &id,
		CreationTime: &t,
		EventTime:    &t,
		StartTime:    &s,
		EndTime:      &t,
		Series: []*TimeSeries{
			&TimeSeries{
				Name: &seriesName,
			},
		},
	}
	err = db.SetSnapshot(snapshot)
	c.Assert(err, IsNil)
}
