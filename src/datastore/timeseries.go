package datastore

import (
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	log "code.google.com/p/log4go"
	"encoding/binary"
	"fmt"
	. "github.com/errplane/errplane-go-common/agent"
	"github.com/jmhodges/levigo"
	"path"
	"strings"
	"sync"
	"time"
	"utils"
)

const (
	KILOBYTES = 1024
	MEGABYTES = 1024 * KILOBYTES
)

type TimeseriesDatastore struct {
	CommonDatastore
}

func NewTimeseriesDatastore(dir string) (*TimeseriesDatastore, error) {
	writeOptions := levigo.NewWriteOptions()
	readOptions := levigo.NewReadOptions()
	datastore := &TimeseriesDatastore{
		CommonDatastore: CommonDatastore{
			dir:          path.Join(dir, "timeseries"),
			writeOptions: writeOptions,
			readOptions:  readOptions,
			readLock:     sync.Mutex{},
		},
	}
	if err := datastore.openDb(time.Now().Unix()); err != nil {
		datastore.Close()
		return nil, utils.WrapInErrplaneError(err)
	}
	return datastore, nil
}

func (self *TimeseriesDatastore) readPoint(database string, series string, id string) (*Point, error) {
	ro := levigo.NewReadOptions()
	defer ro.Close()
	key := fmt.Sprintf("%s~t~%s~%s", database, series, id)
	data, err := self.db.Get(ro, []byte(key))
	if err != nil {
		return nil, utils.WrapInErrplaneError(err)
	}
	if data != nil {
		point := &Point{}
		err := proto.Unmarshal(data, point)
		if err != nil {
			return nil, utils.WrapInErrplaneError(err)
		}
		return point, nil
	}
	return nil, nil
}

func (self *TimeseriesDatastore) updateIndex(database, timeseries string, timestamp time.Time) error {
	// index key
	key := fmt.Sprintf("%s~i~%s",
		database,
		timeseries,
	)

	ro := levigo.NewReadOptions()
	defer ro.Close()

	_value, err := self.db.Get(ro, []byte(key))
	if err != nil {
		return utils.WrapInErrplaneError(err)
	}

	var value int64
	if len(_value) > 0 {
		err = binary.Read(bytes.NewReader(_value), binary.LittleEndian, &value)
		if err != nil {
			return utils.WrapInErrplaneError(err)
		}
	}

	lastUpdate := time.Unix(value, 0)
	if timestamp.Sub(lastUpdate) > 5*time.Minute {
		buffer := bytes.NewBuffer(nil)
		err = binary.Write(buffer, binary.LittleEndian, timestamp.Unix())
		if err != nil {
			return utils.WrapInErrplaneError(err)
		}
		_value = buffer.Bytes()
		err = self.db.Put(self.writeOptions, []byte(key), _value)
		if err != nil {
			return utils.WrapInErrplaneError(err)
		}
	}

	return nil
}

func (self *TimeseriesDatastore) ReadSeriesIndex(database string, limit int64, startTime int64, yield func(string)) error {
	self.readLock.Lock()
	defer self.readLock.Unlock()

	for {
		db, shouldClose, err := self.openDbOrUseTodays(startTime)
		if db == nil || err != nil {
			return err
		}

		if shouldClose {
			defer db.Close()
		}

		ro := levigo.NewReadOptions()
		it := db.NewIterator(ro)
		defer it.Close()
		defer ro.Close()

		beginningKey := fmt.Sprintf("%s~i~", database)
		key := fmt.Sprintf("%s~i~.............................", database)
		if limit <= 0 || limit > 100000 {
			limit = 100000
		}

		it.Seek([]byte(key))

		for it = it; it.Valid() && limit > 0; it.Next() {
			indexKey := string(it.Key())
			if !strings.HasPrefix(indexKey, beginningKey) {
				break
			}
			parts := strings.Split(indexKey, "~")
			if len(parts) > 2 {
				// get the timestamp
				_value, err := db.Get(ro, it.Key())
				if err != nil {
					return utils.WrapInErrplaneError(err)
				}
				var value int64
				if err := binary.Read(bytes.NewReader(_value), binary.LittleEndian, &value); err != nil {
					return utils.WrapInErrplaneError(err)
				}
				if value > startTime {
					yield(parts[2])
				}
			}
			limit--
		}
		if limit == 0 {
			break
		}
		startTime += (24 * int64(time.Hour)) / int64(time.Second)
	}
	return nil
}

func (self *TimeseriesDatastore) ReadSeries(params *GetParams, yield func(*Point) error) error {
	self.readLock.Lock()
	defer self.readLock.Unlock()

	setGetParamsDefaults(params)
	endTime := params.endTime
	for {
		db, shouldClose, err := self.openDbOrUseTodays(endTime)
		if db == nil || err != nil {
			return err
		}

		if shouldClose {
			defer db.Close()
		}

		params.endTime = params.endTime + 1
		ro := levigo.NewReadOptions()
		it := db.NewIterator(ro)
		defer it.Close()
		defer ro.Close()

		beginningKey := fmt.Sprintf("%s~t~%s~", params.database, params.timeSeries)
		key := fmt.Sprintf("%s~t~%s~%d_", params.database, params.timeSeries, params.endTime)

		it.Seek([]byte(key))
		if it.Valid() {
			it.Prev()
		} else {
			log.Info("GET_POINTS: first seek wasn't valid")
		}

		for it = it; it.Valid() && params.limit > 0; it.Prev() {
			pointKey := string(it.Key())
			if !strings.HasPrefix(pointKey, beginningKey) {
				break
			}
			newPoint := &Point{}
			err := proto.Unmarshal(it.Value(), newPoint)
			if err != nil {
				return utils.WrapInErrplaneError(err)
			}
			if *newPoint.Time < params.startTime {
				break
			}
			if params.matchesFilters(newPoint) {
				params.limit--
				if err := yield(newPoint); err != nil {
					return utils.WrapInErrplaneError(err)
				}
			}
		}
		if params.limit == 0 || endTime < params.startTime {
			break
		}
		endTime -= 24 * int64(time.Hour) / int64(time.Second)
	}
	return nil
}

func (self *TimeseriesDatastore) WritePoints(database string, timeseries string, points []*Point) error {
	self.readLock.Lock()
	defer self.readLock.Unlock()

	if err := self.updateIndex(database, timeseries, time.Now()); err != nil {
		return utils.WrapInErrplaneError(err)
	}

	for _, point := range points {
		if err := self.openDb(*point.Time); err != nil {
			return utils.WrapInErrplaneError(err)
		}

		key := fmt.Sprintf("%s~t~%s~%d_%d",
			database,
			timeseries,
			*point.Time,
			*point.SequenceNumber,
		)
		id := fmt.Sprintf("%d_%d", *point.Time, *point.SequenceNumber)
		oldPoint, err := self.readPoint(database, timeseries, id)
		if err != nil {
			return utils.WrapInErrplaneError(err)
		}

		newDimensions := make([]*Dimension, 0)
		if oldPoint != nil {

			// update value
			if *point.Value == float64(0) {
				point.Value = oldPoint.Value
			}

			// update context
			if point.Context == nil {
				point.Context = oldPoint.Context
			}
			for _, dim := range oldPoint.Dimensions {
				newVal, hasDimension := point.GetDimensionValue(dim.Name)
				if hasDimension {
					if *newVal != "" {
						dim.Value = newVal
						newDimensions = append(newDimensions, dim)
					}
				} else {
					newDimensions = append(newDimensions, dim)
				}
			}
		}

		if point.Context != nil && *point.Context == "" {
			point.Context = nil
		}

		// update dimensions
		for _, dim := range point.Dimensions {
			if _, hasDimension := point.GetDimensionValue(dim.Name); !hasDimension {
				newDimensions = append(newDimensions, dim)
			}
		}
		point.Dimensions = newDimensions

		data, err := proto.Marshal(point)
		if err != nil {
			return utils.WrapInErrplaneError(err)
		}
		err = self.db.Put(self.writeOptions, []byte(key), data)
		if err != nil {
			return utils.WrapInErrplaneError(err)
		}
	}
	return nil
}