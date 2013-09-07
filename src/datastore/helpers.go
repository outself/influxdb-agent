package datastore

import (
	"time"
)

func timeToEpoch(timestamp time.Time) int64 {
	return timestamp.Unix()
}

func epochToDay(epoch int64) string {
	return time.Unix(epoch, 0).Format("20060102")
}
