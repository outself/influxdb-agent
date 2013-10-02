package datastore

import (
	"code.google.com/p/goprotobuf/proto"
	log "code.google.com/p/log4go"
	protocol "github.com/errplane/errplane-go-common/agent"
	"github.com/nu7hatch/gouuid"
	"path"
	"regexp"
	"sync"
	"time"
	"utils"
)

type SnapshotDatastore struct {
	CommonDatastore
	database            string
	timeseriesDatastore *TimeseriesDatastore
	SnapshotsLimit      int
}

type SnapshotRequest struct {
	Regex     string
	StartTime int64
	EndTime   int64
	Limit     int64
}

func NewSnapshotDatastore(dir string, database string, timeseriesDatastore *TimeseriesDatastore) (*SnapshotDatastore, error) {
	datastore := &SnapshotDatastore{
		CommonDatastore: CommonDatastore{
			dir:      path.Join(dir, "snapshots"),
			readLock: sync.Mutex{},
		},
		timeseriesDatastore: timeseriesDatastore,
		database:            database,
	}
	// don't use one file per day
	if err := datastore.openDb(-1); err != nil {
		datastore.Close()
		return nil, utils.WrapInErrplaneError(err)
	}
	datastore.SnapshotsLimit = 100
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

func (self *SnapshotDatastore) RemoveOldSnapshots() error {
	itr := self.db.NewIterator(self.readOptions)
	defer itr.Close()
	itr.SeekToFirst()

	oldestId := []byte{}
	oldestTimeStamp := time.Now().Unix()
	count := 0

	for ; itr.Valid(); itr.Next() {
		key := itr.Key()
		if value := itr.Value(); len(value) > 0 {
			count++
			snapshot := &protocol.Snapshot{}
			err := proto.Unmarshal(value, snapshot)
			if err != nil {
				return err
			}
			if *snapshot.CreationTime < oldestTimeStamp {
				oldestTimeStamp = *snapshot.CreationTime
				oldestId = key
			}
		}
	}

	if count > self.SnapshotsLimit {
		if err := self.db.Delete(self.writeOptions, oldestId); err != nil {
			return err
		}
	}
	return nil
}

func (self *SnapshotDatastore) TakeSnapshot(snapshotRequests []*SnapshotRequest) (*protocol.Snapshot, error) {
	if len(snapshotRequests) == 0 {
		return nil, nil
	}
	metrics := map[string]*SnapshotRequest{}
	foo := func(metricName string) {
		for _, request := range snapshotRequests {
			matches, err := regexp.MatchString(request.Regex, metricName)
			if err != nil {
				log.Error("Error while using regex %s. Error: %s", request.Regex, err)
				continue
			}
			if matches {
				metrics[metricName] = request
				break
			}
		}
	}
	self.timeseriesDatastore.ReadSeriesIndex(self.database, snapshotRequests[0].StartTime, foo)
	uuid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	snapshotId := uuid.String()
	creationTime := time.Now().Unix()
	allTimeseries := make([]*protocol.TimeSeries, 0)
	for metric, snapshotRequest := range metrics {
		timeseries := make([]*protocol.Point, 0)
		read := func(p *protocol.Point) error {
			timeseries = append(timeseries, p)
			return nil
		}
		limit := int64(1000)
		if snapshotRequest.Limit > 0 {
			limit = snapshotRequest.Limit
		}
		params := &GetParams{
			Database:   self.database,
			TimeSeries: metric,
			StartTime:  snapshotRequest.StartTime,
			Limit:      limit,
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
		StartTime:    &snapshotRequests[0].StartTime,
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

	return self.RemoveOldSnapshots()
}
