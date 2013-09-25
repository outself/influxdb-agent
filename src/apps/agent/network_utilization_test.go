package main

import (
	log "code.google.com/p/log4go"
	. "launchpad.net/gocheck"
)

type NetworkUtilizationSuite struct{}

var _ = Suite(&NetworkUtilizationSuite{})

/* Mocks */

func (self *NetworkUtilizationSuite) TestParsingCentos(c *C) {
	data := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:   26640     444    0    0    0     0          0         0    26640     444    0    0    0     0       0          0
  eth0:49618425   58443    0    0    0     0          0         0  2984347   31901    0    0    0     0       0          0
`

	utilization := NetworkUtilization{}
	c.Assert(utilization.Parse([]byte(data)), IsNil)
	log.Info("Parsed: %v", utilization)
	c.Assert(utilization["eth0"], NotNil)
	c.Assert(utilization["lo"], NotNil)
}

func (self *NetworkUtilizationSuite) TestParsing(c *C) {
	utilization := NetworkUtilization{}
	c.Assert(utilization.Get(), IsNil)

	log.Info("Parsed: %v", utilization)

	var rxBytes, txBytes int64

	for _, nicUtilization := range utilization {
		rxBytes += nicUtilization.rxBytes
		txBytes += nicUtilization.txBytes
	}
	c.Assert(rxBytes > 0, Equals, true) // there must be an interface that has data sent and received
	c.Assert(txBytes > 0, Equals, true) // there must be an interface that has data sent and received
}
