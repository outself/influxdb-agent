package datastore

import (
	"code.google.com/p/goprotobuf/proto"
	protocol "github.com/errplane/errplane-go-common/agent"
	"github.com/jmhodges/levigo"
	"path"
	"sync"
	"utils"
)

type SnapshotDatastore struct {
	CommonDatastore
}

func NewSnapshotDatastore(dir string) (*SnapshotDatastore, error) {
	writeOptions := levigo.NewWriteOptions()
	readOptions := levigo.NewReadOptions()
	datastore := &SnapshotDatastore{
		CommonDatastore: CommonDatastore{
			dir:          path.Join(dir, "snapshots"),
			writeOptions: writeOptions,
			readOptions:  readOptions,
			readLock:     sync.Mutex{},
		},
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

	snapshot := &protocol.Snapshot{}
	err = proto.Unmarshal(data, snapshot)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
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
