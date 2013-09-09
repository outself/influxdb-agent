package main

import (
	log "code.google.com/p/log4go"
	"github.com/errplane/errplane-go"
	common "github.com/errplane/errplane-go-common"
	"github.com/errplane/errplane-go-common/aggregator"
	"time"
)

type AggregatorConfig struct {
	UdpPort       string
	ApiKey        string
	FlushInterval time.Duration
	Percentiles   []float64
}

// convert between errplane.WriteOperation and common.WriteOperation. Although
// they are the same structures go won't let me assign one to the other.
//
// FIXME: we should unify the two structs so we don't have to do this stupid conversion
func convertToInternalWriteOperation(operation *common.WriteOperation) *errplane.WriteOperation {
	writes := make([]*errplane.JsonPoints, 0, len(operation.Writes))
	for _, write := range operation.Writes {
		points := make([]*errplane.JsonPoint, 0, len(write.Points))
		for _, point := range write.Points {
			points = append(points, &errplane.JsonPoint{
				Value:      point.Value,
				Context:    point.Context,
				Time:       point.Time,
				Dimensions: point.Dimensions,
			})
		}

		writes = append(writes, &errplane.JsonPoints{
			Name:   write.Name,
			Points: points,
		})
	}

	return &errplane.WriteOperation{
		Database:  operation.Database,
		ApiKey:    operation.ApiKey,
		Operation: operation.Operation,
		Writes:    writes,
	}
}

func (self *Agent) handler(operation *common.WriteOperation) {
	if err := self.ep.SendHttp(convertToInternalWriteOperation(operation)); err != nil {
		log.Error("Cannot send data to the Errplane. Error: %s", err)
	}
}

func (self *Agent) startUdpListener() {
	log.Info("Starting data aggregator...")
	theAggregator := aggregator.NewAggregator(self.config.FlushInterval/time.Second, self.handler, self.config.ApiKey, self.config.Percentiles, true)
	udpReceiver := aggregator.NewUdpReceiver(self.config.UdpAddr, self.handler, theAggregator)
	udpReceiver.ListenAndReceive()
}
