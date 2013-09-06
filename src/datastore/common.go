package datastore

import (
	"github.com/jmhodges/levigo"
	"os"
	"path"
	"sync"
	"utils"
)

type CommonDatastore struct {
	day          string
	db           *levigo.DB
	dir          string
	writeOptions *levigo.WriteOptions
	readOptions  *levigo.ReadOptions
	readLock     sync.Mutex
}

func (self *CommonDatastore) openDb(epoch int64) error {
	day := epochToDay(epoch)
	if day == self.day {
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

func (self *CommonDatastore) openLevelDb(day string, createIfMissing bool) (*levigo.DB, error) {
	dir := path.Join(self.dir, day)
	err := os.MkdirAll(self.dir, 0755)
	if err != nil {
		return nil, err
	}
	opts := levigo.NewOptions()
	opts.SetCache(levigo.NewLRUCache(100 * MEGABYTES))
	opts.SetCreateIfMissing(createIfMissing)
	opts.SetBlockSize(256 * KILOBYTES)
	db, err := levigo.Open(dir, opts)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return nil, utils.WrapInErrplaneError(err)
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
	return db, nil
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
