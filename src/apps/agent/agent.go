package main

import (
	"fmt"
	"github.com/cloudfoundry/gosigar"
	"github.com/errplane/errplane-go"
	"os"
	"time"
)

var hostname string

func init() {
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		fmt.Printf("Cannot determine hostname. Error: %s\n", err)
		os.Exit(1)
	}
}

func main() {
	ep := errplane.New("w.apiv3.errplane.com", "udp.apiv3.errplane.com", "app4you2love", "staging", "962cdc9b-15e7-4b25-9a0d-24a45cfc6bc1")
	ch := make(chan error)
	go memStats(ep, ch)
	fmt.Printf("Server started successfully\n")
	err := <-ch
	fmt.Printf("Data collection stopped unexpectedly. Error: %s\n", err)
	return
}

func memStats(ep *errplane.Errplane, ch chan error) {
	mem := sigar.Mem{}
	swap := sigar.Swap{}

	for {
		mem.Get()
		swap.Get()

		err := ep.Report("server.stats.memory.free", float64(mem.Free), time.Now(), "", errplane.Dimensions{
			"host": hostname,
		})
		if err != nil {
			ch <- err
			return
		}
		err = ep.Report("server.stats.memory.used", float64(mem.Used), time.Now(), "", errplane.Dimensions{
			"host": hostname,
		})
		if err != nil {
			ch <- err
			return
		}
		time.Sleep(10 * time.Second)
	}
}
