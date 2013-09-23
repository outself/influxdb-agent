package datastore

import (
	"fmt"
	protocol "github.com/errplane/errplane-go-common/agent"
	. "github.com/errplane/errplane-go-common/agent"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"path"
	"strings"
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

func (self *DatastoreSuite) TestOldDirectoryDelete(c *C) {
	dirName := path.Join(self.dbDir, "timeseries", "20110101")
	err := os.MkdirAll(dirName, 0755)
	c.Assert(err, IsNil)
	db, err := NewTimeseriesDatastore(self.dbDir)
	defer db.Close()
	c.Assert(err, IsNil)
	time.Sleep(1 * time.Second)
	info, err := os.Stat(dirName)
	c.Assert(err, NotNil)
	c.Assert(info, IsNil)
	c.Assert(os.IsNotExist(err), Equals, true)
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

	snapshot1, err := db.TakeSnapshot([]*SnapshotRequest{&SnapshotRequest{Regex: ".*", StartTime: startTime.Unix()}})
	c.Assert(err, IsNil)
	snapshot2, err := db.TakeSnapshot([]*SnapshotRequest{&SnapshotRequest{Regex: ".*", StartTime: startTime.Unix()}})
	c.Assert(err, IsNil)
	fmt.Printf("snapshot1.Id = %s, snapshot2.Id = %s\n", *snapshot1.Id, *snapshot2.Id)
	c.Assert(*snapshot1.Id, Not(Equals), *snapshot2.Id)
}

func (self *DatastoreSuite) TestDeletingOldSnapshots(c *C) {
	timeseriesDb, err := NewTimeseriesDatastore(self.dbDir)
	c.Assert(err, IsNil)

	database := "app4you2loveproduction"
	db, err := NewSnapshotDatastore(self.dbDir, database, timeseriesDb)
	c.Assert(err, IsNil)
	c.Assert(db, NotNil)

	db.SnapshotsLimit = 1

	start := time.Now().Add(-1 * time.Hour).Unix()
	firstSnapshot, err := db.TakeSnapshot([]*SnapshotRequest{&SnapshotRequest{Regex: ".*", StartTime: start}})
	c.Assert(err, IsNil)

	time.Sleep(1 * time.Second)

	_, err = db.TakeSnapshot([]*SnapshotRequest{&SnapshotRequest{Regex: ".*", StartTime: start}})
	c.Assert(err, IsNil)

	snapshot, err := db.GetSnapshot(*firstSnapshot.Id)
	c.Assert(err, IsNil)
	c.Assert(snapshot, IsNil)
}

func (self *DatastoreSuite) TestBenchmarkSnapshotTaking(c *C) {
	timeseriesDb, err := NewTimeseriesDatastore(self.dbDir)
	c.Assert(err, IsNil)

	start := time.Now().Add(-1 * time.Hour)
	// write an hour worth of points for 100 metrics
	for i := 0; i < 100; i++ {
		points := []*Point{}
		for j := 0; j < 60*10; j++ {
			t := start.Add(time.Duration(j) * time.Second).Unix()
			v := 1.0
			var seq uint32 = 0
			points = append(points, &Point{
				Time:           &t,
				Value:          &v,
				SequenceNumber: &seq,
				Context:        nil,
				Dimensions:     nil,
			})
		}
		err = timeseriesDb.WritePoints("app4you2loveproduction", fmt.Sprintf("timeseries%d", i), points)
		c.Assert(err, IsNil)
	}

	database := "app4you2loveproduction"
	db, err := NewSnapshotDatastore(self.dbDir, database, timeseriesDb)
	c.Assert(err, IsNil)
	c.Assert(db, NotNil)

	for i := 0; i < 10; i++ {
		snapshot, err := db.TakeSnapshot([]*SnapshotRequest{&SnapshotRequest{Regex: ".*", StartTime: start.Unix()}})
		c.Assert(err, IsNil)
		c.Assert(snapshot.Series, HasLen, 100)
		c.Assert(snapshot.Series[0].Points, HasLen, 60*10)
	}

	path := path.Join(self.dbDir, "snapshots")
	cmd := exec.Command("sh", "-c", fmt.Sprintf("du -sch %s | grep total", path))
	output, err := cmd.Output()
	c.Assert(err, IsNil)
	_size := strings.Fields(string(output))[0]
	// size, err := strconv.Atoi(_size)
	// c.Assert(err, IsNil)
	fmt.Printf("size of %s is %s\n", path, _size)
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

	snapshot, err := db.TakeSnapshot([]*SnapshotRequest{&SnapshotRequest{Regex: ".*", StartTime: startTime.Unix()}})
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

/*
Notes on how much space this takes:

3.6 million values split across 180 time series took up about 121MB ok disk

*/
// func (self *DatastoreSuite) TestWritingATonOfData(c *C) {
// 	ds, err := NewTimeseriesDatastore("/tmp/agent_test_db")
// 	c.Assert(err, IsNil)
// 	database := "app4you2loveproduction"
// 	host := "pivotal-guest-5-71.ny.pivotallabs.com."
// 	seriesNames := []string{"plugins.mysql.innodb_log_write_requests", "plugins.redis.blocked_clients", "processes.161.cpu.top", "processes.3423.cpu.top", "stats.swap.free", "plugins.mysql.com_stmt_close", "plugins.mysql.innodb_dblwr_writes", "stats.network.eth0.rxBytes", "stats.network.lo.txPackets", "plugins.mongo.status", "plugins.mysql.innodb_page_size", "plugins.mysql.threads_cached", "processes.16226.mem.top", "processes.6310.cpu.top", "plugins.mysql.key_blocks_unused", "plugins.mysql.threads_connected", "stats.cpu.softirq", "plugins.mysql.com_prepare_sql", "plugins.mysql.key_write_requests", "processes.25825.cpu.top", "processes.837.cpu.top", "plugins.mysql.com_commit", "plugins.mysql.com_insert", "plugins.mysql.innodb_data_written", "plugins.mysql.innodb_rows_inserted", "plugins.mysql.opened_table_definitions", "plugins.mysql.com_create_procedure", "plugins.mysql.com_drop_table", "plugins.mysql.com_flush", "plugins.mysql.handler_write", "plugins.mysql.innodb_data_read", "stats.cpu.wait", "stats.network.eth0.txDropped", "plugins.mysql.com_dealloc_sql", "plugins.mysql.com_update_multi", "plugins.redis.total_connections_received", "processes.3.cpu.top", "processes.627.cpu.top", "stats.cpu.irq", "stats.memory.used", "plugins.mysql.com_drop_db", "plugins.mysql.com_execute_sql", "plugins.mysql.created_tmp_disk_tables", "plugins.mysql.qcache_hits", "plugins.mysql.select_range", "processes.627.mem.top", "stats.swap.used_percentage", "plugins.mysql.com_begin", "plugins.mysql.innodb_rows_read", "plugins.mysql.uptime_since_flush_status", "stats.disk./proc.used_percentage", "stats.network.eth0.rxErrors", "plugins.mysql.com_stmt_execute", "plugins.mysql.max_used_connections", "plugins.redis.used_memory", "stats.network.lo.txDropped", "plugins.elasticsearch.status", "plugins.memcached.status", "plugins.mysql.com_show_variables", "plugins.mysql.handler_read_key", "plugins.mysql.opened_files", "plugins.mysql.threads_running", "stats.network.eth0.txBytes", "plugins.mysql.com_set_option", "plugins.mysql.handler_savepoint", "plugins.mysql.innodb_buffer_pool_reads", "plugins.mysql.innodb_pages_created", "stats.disk./.used", "plugins.mysql.handler_savepoint_rollback", "processes.16226.cpu.top", "stats.disk./.used_percentage", "stats.network.eth0.txPackets", "stats.network.lo.rxErrors", "plugins.mysql.com_insert_select", "plugins.mysql.handler_commit", "plugins.mysql.innodb_log_writes", "plugins.mysql.innodb_os_log_written", "plugins.mysql.qcache_free_memory", "plugins.mysql.uptime", "plugins.mysql.com_alter_table", "plugins.mysql.com_show_fields", "plugins.mysql.innodb_pages_written", "plugins.redis.total_commands_processed", "processes.25825.mem.top", "stats.network.eth0.rxPackets", "stats.network.lo.txBytes", "stats.swap.used", "plugins.mysql.com_select", "plugins.mysql.com_show_status", "plugins.mysql.handler_delete", "plugins.mysql.handler_read_rnd_next", "plugins.mysql.status", "plugins.mysql.threads_created", "stats.cpu.stolen", "stats.memory.used_percentage", "plugins.mysql.com_update", "plugins.mysql.opened_tables", "plugins.redis.db0.keys", "processes.1383.cpu.top", "stats.cpu.idle", "plugins.mysql.com_stmt_prepare", "plugins.mysql.innodb_buffer_pool_pages_data", "plugins.mysql.innodb_buffer_pool_pages_misc", "plugins.mysql.select_scan", "plugins.mysql.slave_heartbeat_period", "plugins.mysql.com_change_db", "plugins.mysql.com_drop_procedure", "plugins.mysql.com_release_savepoint", "plugins.mysql.innodb_dblwr_pages_written", "plugins.mysql.table_locks_immediate", "stats.network.eth0.txErrors", "plugins.apache.status", "plugins.mysql.bytes_sent", "plugins.mysql.com_check", "plugins.mysql.innodb_buffer_pool_pages_free", "plugins.mysql.innodb_buffer_pool_pages_total", "plugins.mysql.innodb_data_fsyncs", "plugins.mysql.innodb_pages_read", "plugins.mysql.open_table_definitions", "plugins.mysql.qcache_inserts", "stats.cpu.sys", "plugins.mysql.bytes_received", "plugins.mysql.handler_read_next", "plugins.mysql.innodb_buffer_pool_pages_flushed", "plugins.mysql.qcache_queries_in_cache", "plugins.mysql.com_rollback_to_savepoint", "plugins.mysql.com_show_keys", "plugins.mysql.open_files", "processes.1383.mem.top", "plugins.mysql.com_create_db", "plugins.mysql.flush_commands", "plugins.mysql.last_query_cost", "plugins.mysql.questions", "plugins.postgres.status", "plugins.redis.status", "stats.io.xvda3", "plugins.mysql.created_tmp_files", "plugins.mysql.created_tmp_tables", "stats.cpu.user", "stats.disk./proc.used", "stats.memory.actual_used", "stats.memory.free", "stats.network.lo.rxDropped", "plugins.mysql.aborted_connects", "plugins.mysql.com_show_create_table", "plugins.mysql.innodb_buffer_pool_write_requests", "plugins.mysql.innodb_os_log_fsyncs", "plugins.mysql.key_read_requests", "processes.21.cpu.top", "processes.6310.mem.top", "stats.io.xvda1", "plugins.mysql.com_admin_commands", "plugins.mysql.handler_read_first", "plugins.mysql.innodb_buffer_pool_read_requests", "plugins.mysql.key_writes", "plugins.mysql.open_tables", "plugins.mysql.queries", "plugins.nginx.status", "plugins.mysql.com_create_index", "plugins.mysql.connections", "plugins.mysql.handler_rollback", "plugins.mysql.innodb_buffer_pool_bytes_data", "plugins.mysql.innodb_data_reads", "plugins.mysql.qcache_free_blocks", "plugins.mysql.qcache_total_blocks", "plugins.redis.connected_clients", "stats.network.lo.rxPackets", "plugins.mysql.com_rollback", "plugins.mysql.com_show_databases", "plugins.mysql.key_blocks_used", "stats.network.eth0.rxDropped", "plugins.mysql.com_savepoint", "stats.network.lo.rxBytes", "plugins.mysql.innodb_data_writes", "plugins.mysql.qcache_not_cached", "plugins.mysql.com_create_table", "plugins.mysql.com_show_tables", "plugins.mysql.select_full_join", "plugins.redis.uptime_in_seconds", "stats.network.lo.txErrors"}
// 	startTime := time.Now().Unix()/86400*86400 + 1
// 	pointsWritten := 0
// 	for i := 0; i < 20000; i++ {
// 		for _, n := range seriesNames {
// 			v := rand.Float64() * float64(i)
// 			err := ds.WritePoints(database, host+n, []*protocol.Point{&protocol.Point{Value: &v, Time: &startTime}})
// 			c.Assert(err, IsNil)
// 			pointsWritten += 1
// 		}
// 		startTime += 1
// 	}
// 	fmt.Printf("Points written to test db: %d\nSeries names written to test db: %d", pointsWritten, len(seriesNames))
// }
