package datastore

import (
	"fmt"
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

	// make sure the index is updated
	metrics := map[string]bool{}
	err = db.ReadSeriesIndex("dbname", 0, timestamps[0], func(m string) {
		metrics[m] = true
	})
	c.Assert(metrics, HasLen, 1)
	c.Assert(metrics["timeseries"], Equals, true)

	// make sure the points are inserted
	points := make([]*Point, 0)
	err = db.ReadSeries(&GetParams{
		Database:   "dbname",
		TimeSeries: "timeseries",
		StartTime:  timestamps[0],
		EndTime:    timestamps[len(timestamps)-1],
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
	timeseriesDb, err := NewTimeseriesDatastore(self.dbDir)
	c.Assert(err, IsNil)
	db, err := NewSnapshotDatastore(self.dbDir, "", timeseriesDb)
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

func (self *DatastoreSuite) TestSnapshotIdUniqueness(c *C) {
	timeseriesDb, err := NewTimeseriesDatastore(self.dbDir)
	c.Assert(err, IsNil)
	database := "app4you2loveproduction"
	db, err := NewSnapshotDatastore(self.dbDir, database, timeseriesDb)
	c.Assert(err, IsNil)
	c.Assert(db, NotNil)

	startTime := time.Now().Add(-5 * time.Minute)

	snapshot1, err := db.TakeSnapshot([]string{".*"}, startTime)
	c.Assert(err, IsNil)
	snapshot2, err := db.TakeSnapshot([]string{".*"}, startTime)
	c.Assert(err, IsNil)
	fmt.Printf("snapshot1.Id = %s, snapshot2.Id = %s\n", *snapshot1.Id, *snapshot2.Id)
	c.Assert(*snapshot1.Id, Not(Equals), *snapshot2.Id)
}

func (self *DatastoreSuite) TestSnapshotTaking(c *C) {
	timeseriesDb, err := NewTimeseriesDatastore(self.dbDir)
	c.Assert(err, IsNil)
	database := "app4you2loveproduction"
	db, err := NewSnapshotDatastore(self.dbDir, database, timeseriesDb)
	c.Assert(err, IsNil)
	c.Assert(db, NotNil)

	startTime := time.Now().Add(-5 * time.Minute)

	pointTime := startTime.Add(2 * time.Minute).Unix()
	var sequence uint32 = 1
	value := 1.0
	// insert some data
	points := []*protocol.Point{
		&protocol.Point{
			Time:           &pointTime,
			SequenceNumber: &sequence,
			Value:          &value,
		},
	}
	err = timeseriesDb.WritePoints(database, "timeseries1", points)
	c.Assert(err, IsNil)
	value = 2.0
	err = timeseriesDb.WritePoints(database, "timeseries2", points)
	c.Assert(err, IsNil)

	snapshot, err := db.TakeSnapshot([]string{".*"}, startTime)
	c.Assert(err, IsNil)
	existingSnapshot, err := db.GetSnapshot(snapshot.GetId())
	c.Assert(err, IsNil)

	for _, snapshot := range []*protocol.Snapshot{snapshot, existingSnapshot} {
		c.Assert(snapshot.Series, HasLen, 2)
		c.Assert(*snapshot.Series[0].Name, Equals, "timeseries1")
		c.Assert(snapshot.Series[0].Points, HasLen, 1)
		c.Assert(*snapshot.Series[0].Points[0].Value, Equals, 1.0)
		c.Assert(*snapshot.Series[1].Name, Equals, "timeseries2")
		c.Assert(snapshot.Series[1].Points, HasLen, 1)
		c.Assert(*snapshot.Series[1].Points[0].Value, Equals, 2.0)
	}
}
