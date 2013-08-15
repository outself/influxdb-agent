package main

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
)

type ProcessIO struct {
	Rchar               uint64 // 203933344701
	Wchar               uint64 // 15849091407
	Syscr               uint64 // 53519490
	Syscw               uint64 // 43630538
	ReadBytes           uint64 // 1075021824
	WriteBytes          uint64 // 3787755520
	CancelledWriteBytes uint64 // 1776705536
}

func (self *ProcessIO) Get(pid int) error {
	data, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/io", pid))
	if err != nil {
		return err
	}

	values := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		key := strings.Trim(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return err
		}
		values[key] = value
	}

	self.Rchar = values["rchar"]
	self.Wchar = values["wchar"]
	self.Syscr = values["syscr"]
	self.Syscw = values["syscw"]
	self.ReadBytes = values["read_bytes"]
	self.WriteBytes = values["write_bytes"]
	self.CancelledWriteBytes = values["cancelled_write_bytes"]

	return nil
}
