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
	"time"
	"utils"
)

const (
	KILOBYTES = 1024
	MEGABYTES = 1024 * KILOBYTES
)

type TimeseriesDatastore struct {
	day          string
	db           *levigo.DB
	today        string
	dir          string
	writeOptions *levigo.WriteOptions
	readOptions  *levigo.ReadOptions
}

func NewTimeseriesDatastore(dir string) (*TimeseriesDatastore, error) {
	writeOptions := levigo.NewWriteOptions()
	readOptions := levigo.NewReadOptions()
	datastore := &TimeseriesDatastore{
		dir:          dir,
		writeOptions: writeOptions,
		readOptions:  readOptions,
	}
	if err := datastore.openDb(time.Now().Unix()); err != nil {
		datastore.Close()
		return nil, utils.WrapInErrplaneError(err)
	}
	return datastore, nil
}

func (self *TimeseriesDatastore) openDb(epoch int64) error {
	day := epochToDay(epoch)
	if day != self.day {
		dir := path.Join(self.dir, day)
		opts := levigo.NewOptions()
		opts.SetCache(levigo.NewLRUCache(100 * MEGABYTES))
		opts.SetCreateIfMissing(true)
		opts.SetBlockSize(256 * KILOBYTES)
		db, err := levigo.Open(dir, opts)
		if err != nil {
			return utils.WrapInErrplaneError(err)
		}

		// this initializes the ends of the keyspace so seeks don't mess with us.
		db.Put(self.writeOptions, []byte("9999"), []byte(""))
		db.Put(self.writeOptions, []byte("0000"), []byte(""))
		db.Put(self.writeOptions, []byte("aaaa"), []byte(""))
		db.Put(self.writeOptions, []byte("zzzz"), []byte(""))
		db.Put(self.writeOptions, []byte("AAAA"), []byte(""))
		db.Put(self.writeOptions, []byte("ZZZZ"), []byte(""))
		db.Put(self.writeOptions, []byte("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"), []byte(""))
		db.Put(self.writeOptions, []byte("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~"), []byte(""))

		self.day = day
		self.db = db
	}
	return nil
}

func (self *TimeseriesDatastore) Close() {
	self.writeOptions.Close()
	self.readOptions.Close()
	if self.db != nil {
		self.db.Close()
	}
	self.db = nil
}

func (self *TimeseriesDatastore) ReadPoint(database string, series string, id string) (*Point, error) {
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

func (self *TimeseriesDatastore) GetDimensionValue(point *Point, dimensionName *string) (value *string, hasDimension bool) {
	var dim *Dimension
	for _, d := range point.Dimensions {
		if *d.Name == *dimensionName {
			dim = d
			break
		}
	}
	if dim != nil {
		return dim.Value, true
	}
	d := ""
	return &d, false
}

type GetParams struct {
	database          string
	timeSeries        string
	startTime         int64
	endTime           int64
	limit             int64
	includeContext    bool
	includeDimensions bool
	includeIds        bool
	filter            map[string]string
	notFilter         map[string]string
}

func (self *TimeseriesDatastore) ReadSeriesIndex(database string, limit int64, startTime int64, yield func(string)) error {
	ro := levigo.NewReadOptions()
	it := self.db.NewIterator(ro)
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
			_value, err := self.db.Get(ro, it.Key())
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
	return nil
}

func (self *TimeseriesDatastore) matchesFilters(params *GetParams, point *Point) bool {
	shouldFilter := len(params.filter) > 0 || len(params.notFilter) > 0
	if shouldFilter {
		for dimensionName, expectedValue := range params.filter {
			val, hasDimension := self.GetDimensionValue(point, &dimensionName)
			if !hasDimension || *val != expectedValue {
				return false
			}
		}
		for dimensionName, expectedValue := range params.notFilter {
			val, _ := self.GetDimensionValue(point, &dimensionName)
			if *val == expectedValue {
				return false
			}
		}
	}
	return true
}

func setGetParamsDefaults(params *GetParams) {
	if params.endTime == 0 {
		params.endTime = time.Now().Unix()
	}

	if params.limit == 0 {
		params.limit = 50000
	}
}

func (self *TimeseriesDatastore) ReadSeries(params *GetParams, yield func(*Point) error) error {
	setGetParamsDefaults(params)
	params.endTime = params.endTime + 1
	ro := levigo.NewReadOptions()
	it := self.db.NewIterator(ro)
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
		if self.matchesFilters(params, newPoint) {
			params.limit--
			if err := yield(newPoint); err != nil {
				return utils.WrapInErrplaneError(err)
			}
		}
	}
	return nil
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
	if lastUpdate.Sub(timestamp) > 5*time.Minute {
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

func timeToEpoch(timestamp time.Time) int64 {
	return timestamp.Unix()
}

func epochToDay(epoch int64) string {
	return time.Unix(epoch, 0).Format("20060102")
}

func (self *TimeseriesDatastore) WritePoints(database string, timeseries string, points []*Point) error {
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
		oldPoint, err := self.ReadPoint(database, timeseries, id)
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
				newVal, hasDimension := self.GetDimensionValue(point, dim.Name)
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
			if _, hasDimension := self.GetDimensionValue(oldPoint, dim.Name); !hasDimension {
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
