package main

import (
	log "code.google.com/p/log4go"
	"github.com/errplane/gosigar"
	"time"
)

type ProcStat struct {
	pid    int
	now    time.Time
	cpu    sigar.ProcTime
	memory sigar.ProcMem
	state  sigar.ProcState
	args   sigar.ProcArgs
}

func getProcStat(pid int) (*ProcStat, error) {
	now := time.Now()
	state := sigar.ProcState{}
	mem := sigar.ProcMem{}
	procTime := sigar.ProcTime{}
	procArg := sigar.ProcArgs{}

	if err := state.Get(pid); err != nil {
		return nil, err
	}
	if err := mem.Get(pid); err != nil {
		return nil, err
	}
	if err := procTime.Get(pid); err != nil {
		return nil, err
	}
	if err := procArg.Get(pid); err != nil {
		return nil, err
	}

	return &ProcStat{pid, now, procTime, mem, state, procArg}, nil
}

type MergedProcStat struct {
	pid      int
	name     string
	args     []string
	cpuUsage float64
	memUsage float64
}

func mergeStats(old, current map[int]*ProcStat) []MergedProcStat {
	mergedStat := make([]MergedProcStat, 0, len(current))

	for pid, newStat := range current {
		oldStat, ok := old[pid]
		if !ok {
			continue
		}

		if newStat.now.Before(oldStat.now) {
			// skip the process, and may be log this as info
			log.Warn("Possibly a bug, time of new snapshot is less than time of the old snapshot")
			continue
		}

		if newStat.cpu.Total < oldStat.cpu.Total || newStat.state.Name != oldStat.state.Name {
			log.Info("A new process seems to have stolen the pid of an old process")
			continue
		}

		uptime := newStat.now.Sub(oldStat.now).Nanoseconds() / int64(time.Millisecond)
		cpuUsage := float64(newStat.cpu.Total-oldStat.cpu.Total) / float64(uptime) * 100
		memUsage := float64(newStat.memory.Resident)
		mergedStat = append(mergedStat, MergedProcStat{newStat.pid, newStat.state.Name, newStat.args.List, cpuUsage, memUsage})
	}
	return mergedStat
}

func (self *ProcStat) CpuUsage() float64 {
	return 0.0
}

func (self *ProcStat) MemUsage() float64 {
	return float64(self.memory.Size)
}

type ProcStatsSortableByCpu []MergedProcStat
type ProcStatsSortableByMem []MergedProcStat

func (self ProcStatsSortableByCpu) Len() int           { return len(self) }
func (self ProcStatsSortableByCpu) Swap(i, j int)      { self[i], self[j] = self[j], self[i] }
func (self ProcStatsSortableByCpu) Less(i, j int) bool { return self[i].cpuUsage > self[j].cpuUsage }

func (self ProcStatsSortableByMem) Len() int           { return len(self) }
func (self ProcStatsSortableByMem) Swap(i, j int)      { self[i], self[j] = self[j], self[i] }
func (self ProcStatsSortableByMem) Less(i, j int) bool { return self[i].memUsage > self[j].memUsage }
