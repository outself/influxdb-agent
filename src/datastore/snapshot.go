package datastore

import (
	"code.google.com/p/goprotobuf/proto"
	log "code.google.com/p/log4go"
	protocol "github.com/errplane/errplane-go-common/agent"
	"github.com/jmhodges/levigo"
	"github.com/nu7hatch/gouuid"
	"path"
	"regexp"
	"sync"
	"time"
	"utils"
)

type SnapshotDatastore struct {
	CommonDatastore
	timeseriesDatastore *TimeseriesDatastore
}

func NewSnapshotDatastore(dir string, timeseriesDatastore *TimeseriesDatastore) (*SnapshotDatastore, error) {
	writeOptions := levigo.NewWriteOptions()
	readOptions := levigo.NewReadOptions()
	datastore := &SnapshotDatastore{
		CommonDatastore: CommonDatastore{
			dir:          path.Join(dir, "snapshots"),
			writeOptions: writeOptions,
			readOptions:  readOptions,
			readLock:     sync.Mutex{},
		},
		timeseriesDatastore: timeseriesDatastore,
	}
	// don't use one file per day
	if err := datastore.openDb(-1); err != nil {
		datastore.Close()
		return nil, utils.WrapInErrplaneError(err)
	}
	return datastore, nil
}

func (self *SnapshotDatastore) GetSnapshot(id string) (*protocol.Snapshot, error) {
	data, err := self.db.Get(self.readOptions, []byte(id))
	if err != nil {
		return nil, err
	}

	if data == nil || len(data) == 0 {
		return nil, nil
	}

	snapshot := &protocol.Snapshot{}
	err = proto.Unmarshal(data, snapshot)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (self *SnapshotDatastore) TakeSnapshot(relatedMetricsRegex []string, start time.Time) (*protocol.Snapshot, error) {
	metrics := map[string]bool{}
	foo := func(metricName string) {
		for _, regex := range relatedMetricsRegex {
			matches, err := regexp.MatchString(regex, metricName)
			if err != nil {
				log.Error("Error while using regex %s. Error: %s", regex, err)
				continue
			}
			if matches {
				metrics[metricName] = true
				break
			}
		}
	}
	self.timeseriesDatastore.ReadSeriesIndex(utils.AgentConfig.Database(), 0, start.Unix(), foo)
	uuid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	snapshotId := uuid.String()
	snapshotId = "foo"
	creationTime := time.Now().Unix()
	startTime := start.Unix()
	allTimeseries := make([]*protocol.TimeSeries, 0)
	for metric, _ := range metrics {
		timeseries := make([]*protocol.Point, 0)
		read := func(p *protocol.Point) error {
			timeseries = append(timeseries, p)
			return nil
		}
		params := &GetParams{
			database:   utils.AgentConfig.Database(),
			timeSeries: metric,
			startTime:  start.Unix(),
		}
		err := self.timeseriesDatastore.ReadSeries(params, read)
		if err != nil {
			return nil, err
		}
		metricName := metric
		allTimeseries = append(allTimeseries, &protocol.TimeSeries{
			Name:   &metricName,
			Points: timeseries,
		})
	}
	snapshot := &protocol.Snapshot{
		Id:           &snapshotId,
		CreationTime: &creationTime,
		EventTime:    &creationTime,
		StartTime:    &startTime,
		EndTime:      &creationTime,
		Series:       allTimeseries,
	}
	return snapshot, self.SetSnapshot(snapshot)
}

func (self *SnapshotDatastore) SetSnapshot(snapshot *protocol.Snapshot) error {
	data, err := proto.Marshal(snapshot)
	if err != nil {
		return err
	}
	err = self.db.Put(self.writeOptions, []byte(snapshot.GetId()), data)
	if err != nil {
		return err
	}

	return nil
}
