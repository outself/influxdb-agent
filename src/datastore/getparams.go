package datastore

import (
	. "github.com/errplane/errplane-go-common/agent"
	"time"
)

type GetParams struct {
	database          string
	timeSeries        string
	startTime         int64
	endTime           int64
	limit             int64
	includeContext    bool
	includeDimensions bool
	includeIds        bool
	filter            map[string]string
	notFilter         map[string]string
}

func (self *GetParams) matchesFilters(point *Point) bool {
	shouldFilter := len(self.filter) > 0 || len(self.notFilter) > 0
	if shouldFilter {
		for dimensionName, expectedValue := range self.filter {
			val, hasDimension := point.GetDimensionValue(&dimensionName)
			if !hasDimension || *val != expectedValue {
				return false
			}
		}
		for dimensionName, expectedValue := range self.notFilter {
			val, _ := point.GetDimensionValue(&dimensionName)
			if *val == expectedValue {
				return false
			}
		}
	}
	return true
}

func setGetParamsDefaults(params *GetParams) {
	if params.endTime == 0 {
		params.endTime = time.Now().Unix()
	}

	if params.limit == 0 {
		params.limit = 50000
	}
}
