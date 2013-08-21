package main

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path"
	"testing"
)

// Hook up gocheck into the gotest runner.
func Test(t *testing.T) {
	TestingT(t)
}

type AgentSuite struct{}

var _ = Suite(&AgentSuite{})

/* Mocks */

type FakeProcessState struct{ ExitCode int }

func (self *FakeProcessState) ExitStatus() int { return self.ExitCode }

/* Tests */

func (self *AgentSuite) TestPluginInfoParsing(c *C) {
	content := `version: 1.0
output: nagios
needs-dependencies: false
calculate-rates:
  - "queries"
  - "com_.*"
  - "handler_.*"
  - "qcache_.*"
  - "table_locks_.*"
`
	dir := path.Join(os.TempDir(), "foobar")
	c.Assert(os.MkdirAll(dir, 0755), IsNil)
	c.Assert(ioutil.WriteFile(path.Join(dir, "info.yml"), []byte(content), 0644), IsNil)
	plugin, err := parsePluginInfo(dir)
	c.Assert(err, IsNil)
	c.Assert(plugin.CalculateRates, HasLen, 5)
}

func (self *AgentSuite) TestNagiosOutputParsing(c *C) {
	msg := "Warning: process not responding"
	output, err := parseNagiosOutput(&FakeProcessState{1}, msg)
	c.Assert(err, IsNil)
	c.Assert(output.state, Equals, WARNING)
	c.Assert(output.msg, Equals, msg)

	msg = "Critical: process not running"
	output, err = parseNagiosOutput(&FakeProcessState{2}, msg)
	c.Assert(err, IsNil)
	c.Assert(output.state, Equals, CRITICAL)
	c.Assert(output.msg, Equals, msg)

	// 'label'=value[UOM];[warn];[crit];[min];[max]
	msg = "Ok: process is running|'foo'=1.0s;warn;;; noquote=100 'withquote'''=500"
	output, err = parseNagiosOutput(&FakeProcessState{0}, msg)
	c.Assert(err, IsNil)
	c.Assert(output.state, Equals, OK)
	c.Assert(output.msg, Equals, "Ok: process is running")
	c.Assert(output.metrics, HasLen, 3)
	c.Assert(output.metrics["foo"], Equals, 1.0)
	c.Assert(output.metrics["noquote"], Equals, 100.0)
	c.Assert(output.metrics["withquote'"], Equals, 500.0)

	// 'label'=value[UOM];[warn];[crit];[min];[max]
	msg = "Ok: process is running|'foo'= noquote=100"
	output, err = parseNagiosOutput(&FakeProcessState{0}, msg)
	c.Assert(err, IsNil)
	c.Assert(output.state, Equals, OK)
	c.Assert(output.msg, Equals, "Ok: process is running")
	c.Assert(output.metrics, HasLen, 1)
	c.Assert(output.metrics["noquote"], Equals, 100.0)

	// test the redis plugin output
	// ran using `~/Downloads/check_redis.pl -H localhost -o 'DISPLAY:NO,PERF:YES,PATTERN:.*'`
	msg = `OK: REDIS 2.6.10 on localhost:6379 has 1 databases (db0) with 3 keys, up 3 days 22 hours | uptime_in_seconds=340305 os=Linux 3.5.0-17-generic x86_64 total_connections_received=1728 used_memory_lua=31744 total_expires=0 used_cpu_sys=210.11 used_memory_rss=2064384 redis_git_dirty=0 loading=0 redis_mode=standalone latest_fork_usec=0 rdb_last_bgsave_time_sec=-1 connected_clients=1 used_memory_peak_human=825.98K run_id=9a2935c83bd8629bbea3d3a3eac789c249333593 rdb_last_bgsave_status=ok uptime_in_days=3 mem_allocator=jemalloc-3.2.0 pubsub_patterns=0 client_biggest_input_buf=0 gcc_version=4.7.2 keyspace_hits=0 arch_bits=64 aof_rewrite_scheduled=0 lru_clock=1231438 rdb_last_save_time=1375122876 rdb_changes_since_last_save=8 role=master multiplexing_api=epoll rdb_bgsave_in_progress=0 db0_expires=0 rejected_connections=0 pubsub_channels=0 redis_git_sha1=5bdd2af3 aof_last_rewrite_time_sec=-1 used_cpu_user_children=0.00 db0_keys=3 used_memory_human=825.94K process_id=30421 aof_current_rewrite_time_sec=-1 keyspace_misses=0 used_cpu_user=277.75 tcp_port=6379 total_commands_processed=1727 mem_fragmentation_ratio=2.44 used_memory=845760 rdb_current_bgsave_time_sec=-1 client_longest_output_list=0 blocked_clients=0 aof_enabled=0 instantaneous_ops_per_sec=0 evicted_keys=0 aof_last_bgrewrite_status=ok total_keys=3 aof_rewrite_in_progress=0 used_memory_peak=845808 expired_keys=0 connected_slaves=0 used_cpu_sys_children=0.00`

	output, err = parseNagiosOutput(&FakeProcessState{0}, msg)
	c.Assert(err, IsNil)
	c.Assert(output.state, Equals, OK)
	c.Assert(output.msg, Equals, "OK: REDIS 2.6.10 on localhost:6379 has 1 databases (db0) with 3 keys, up 3 days 22 hours")
	c.Assert(len(output.metrics), Equals, 47)
	c.Assert(output.metrics["uptime_in_seconds"], Equals, 340305.0)
	c.Assert(output.metrics["total_connections_received"], Equals, 1728.0)
	c.Assert(output.metrics["lru_clock"], Equals, 1231438.0)
}
