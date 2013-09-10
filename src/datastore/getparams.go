package datastore

import (
	. "github.com/errplane/errplane-go-common/agent"
	"time"
)

type GetParams struct {
	Database          string
	TimeSeries        string
	StartTime         int64
	EndTime           int64
	Limit             int64
	IncludeContext    bool
	IncludeDimensions bool
	IncludeIds        bool
	Filter            map[string]string
	NotFilter         map[string]string
}

func (self *GetParams) matchesFilters(point *Point) bool {
	shouldFilter := len(self.Filter) > 0 || len(self.NotFilter) > 0
	if shouldFilter {
		for dimensionName, expectedValue := range self.Filter {
			val, hasDimension := point.GetDimensionValue(&dimensionName)
			if !hasDimension || *val != expectedValue {
				return false
			}
		}
		for dimensionName, expectedValue := range self.NotFilter {
			val, _ := point.GetDimensionValue(&dimensionName)
			if *val == expectedValue {
				return false
			}
		}
	}
	return true
}

func setGetParamsDefaults(params *GetParams) {
	if params.EndTime == 0 {
		params.EndTime = time.Now().Unix()
	}

	if params.Limit == 0 {
		params.Limit = 50000
	}
}
