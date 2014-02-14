package main

import (
	"bufio"
	"bytes"
	//"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"strconv"
	"strings"
)

// http://www.xaprb.com/blog/2010/01/09/how-linux-iostat-computes-its-results/
// https://www.kernel.org/doc/Documentation/iostats.txt

type DiskUsage struct {
	Name            string
	ReadsCompleted  uint64
	ReadsMerged     uint64
	SectorsRead     uint64
	TotalReadTime   uint64
	WritesCompleted uint64
	WritesMerged    uint64
	SectorsWritten  uint64
	TotalWriteTime  uint64
	IOInProgress    uint64
	TotalIOTime     uint64 // in milliseconds
}

func GetDiskUsages() ([]DiskUsage, error) {
	contents, err := ioutil.ReadFile("/proc/diskstats")
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(bytes.NewBuffer(contents))

	diskUsages := make([]DiskUsage, 0)

	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		diskUsage, err := parseDiskUsageLine(string(line))
		if err != nil {
			return nil, err
		}
		diskUsages = append(diskUsages, diskUsage)
	}

	return diskUsages, nil
}

func parseDiskUsageLine(line string) (DiskUsage, error) {
	fields := strings.Fields(line)

	usage := DiskUsage{Name: fields[2]}
	diskUsageValue := reflect.ValueOf(&usage)
	for i := 1; i < diskUsageValue.Elem().NumField(); i++ {
		value := fields[i+2]
		intValue, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return usage, err
		}
		diskUsageValue.Elem().Field(i).SetUint(intValue)
	}

	return usage, nil
}
