package main

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
)

type LoadAverage [3]float64

const (
	LOAD_AVG_FILE = "proc/loadavg"
)

func (self *LoadAverage) Get() error {
	statFile, err := ioutil.ReadFile(LOAD_AVG_FILE)
	if err != nil {
		return err
	}

	lines := strings.Split(string(statFile), "\n")
	if len(lines) <= 1 {
		return fmt.Errorf("%s doesn't have the expected format", LOAD_AVG_FILE)
	}

	fields := strings.Fields(lines[0])
	if len(fields) < 3 {
		return fmt.Errorf("%s doesn't have the expected format", LOAD_AVG_FILE)
	}

	for i := 0; i < len(self); i++ {
		self[i], err = strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return err
		}
	}

	return nil
}
