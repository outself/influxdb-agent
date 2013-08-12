package main

import (
	log "code.google.com/p/log4go"
	. "launchpad.net/gocheck"
)

type NetworkUtilizationSuite struct{}

var _ = Suite(&NetworkUtilizationSuite{})

/* Mocks */

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
