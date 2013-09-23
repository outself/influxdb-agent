package datastore

import (
	"github.com/jmhodges/levigo"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"utils"
)

var GLOBAL_CACHE = levigo.NewLRUCache(10 * MEGABYTES)

type CommonDatastore struct {
	day            string
	db             *levigo.DB
	dir            string
	writeOptions   *levigo.WriteOptions
	readOptions    *levigo.ReadOptions
	readLock       sync.Mutex
	sequenceNumber uint32
}

func (self *CommonDatastore) nextSequenceNumber() uint32 {
	return atomic.AddUint32(&self.sequenceNumber, 1)
}

func (self *CommonDatastore) openDb(epoch int64) error {
	day := ""
	if epoch > 0 {
		day = epochToDay(epoch)
	}
	if day == self.day && day != "" {
		return nil
	}
	db, err := self.openLevelDb(day, true)
	if err != nil {
		return err
	}
	if self.db != nil {
		self.db.Close()
	}
	self.db = db
	self.day = day
	return nil
}

func (self *CommonDatastore) createDummyKey(db *levigo.DB, key string) {
	if value, _ := db.Get(self.readOptions, []byte(key)); value != nil {
		return
	}
	db.Put(self.writeOptions, []byte(key), []byte{})
}

func (self *CommonDatastore) openLevelDb(day string, createIfMissing bool) (*levigo.DB, error) {
	dir := self.dir
	if day != "" {
		dir = path.Join(dir, day)
	}
	err := os.MkdirAll(self.dir, 0755)
	if err != nil {
		return nil, err
	}
	opts := levigo.NewOptions()
	if self.readOptions == nil {
		self.readOptions = levigo.NewReadOptions()
	}
	if self.writeOptions == nil {
		self.writeOptions = levigo.NewWriteOptions()
	}
	opts.SetCache(GLOBAL_CACHE)
	opts.SetCreateIfMissing(createIfMissing)
	opts.SetBlockSize(256 * KILOBYTES)
	db, err := levigo.Open(dir, opts)
	if err != nil {
		return nil, utils.WrapInErrplaneError(err)
	}

	for _, key := range []string{"9999", "0000", "aaaa", "zzzz", "ZZZZ", "AAAA", strings.Repeat("!", 96), strings.Repeat("~", 96)} {
		self.createDummyKey(db, key)
	}
	return db, nil
}

func (self *CommonDatastore) openDbOrUseTodays(t int64) (db *levigo.DB, shouldClose bool, err error) {
	day := epochToDay(t)
	db = self.db
	if day != self.day {
		db, err = self.openLevelDb(day, false)
		if err != nil {
			if strings.Contains(err.Error(), "does not exist") {
				err = nil
			}
			return
		}
		shouldClose = true
	}
	return
}

func (self *CommonDatastore) Close() {
	self.readLock.Lock()
	defer self.readLock.Unlock()

	self.writeOptions.Close()
	self.readOptions.Close()
	if self.db != nil {
		self.db.Close()
	}
	self.db = nil
}
