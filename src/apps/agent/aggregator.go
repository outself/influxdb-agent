package main

import (
	log "code.google.com/p/log4go"
	"github.com/errplane/errplane-go"
	common "github.com/errplane/errplane-go-common"
	"github.com/errplane/errplane-go-common/aggregator"
	"time"
	. "utils"
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

func handler(ep *errplane.Errplane) aggregator.WriteOperationHandler {
	return func(operation *common.WriteOperation) {
		if err := ep.SendHttp(convertToInternalWriteOperation(operation)); err != nil {
			log.Error("Cannot send data to the Errplane. Error: %s", err)
		}
	}
}

func startUdpListener(ep *errplane.Errplane) {
	log.Info("Starting data aggregator...")
	theAggregator := aggregator.NewAggregator(AgentConfig.FlushInterval/time.Second, handler(ep), AgentConfig.ApiKey, AgentConfig.Percentiles, true)
	udpReceiver := aggregator.NewUdpReceiver(AgentConfig.UdpAddr, handler(ep), theAggregator)
	udpReceiver.ListenAndReceive()
}
