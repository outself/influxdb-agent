package main

import (
	"github.com/cloudfoundry/gosigar"
	"time"
)

type ProcStat struct {
	pid    int
	now    time.Time
	cpu    sigar.ProcTime
	memory sigar.ProcMem
	state  sigar.ProcState
}

func getProcStat(pid int) *ProcStat {
	now := time.Now()
	state := sigar.ProcState{}
	mem := sigar.ProcMem{}
	procTime := sigar.ProcTime{}

	if err := state.Get(pid); err != nil {
		return nil
	}
	if err := mem.Get(pid); err != nil {
		return nil
	}
	if err := procTime.Get(pid); err != nil {
		return nil
	}

	return &ProcStat{pid, now, procTime, mem, state}
}

type MergedProcStat struct {
	pid      int
	name     string
	cpuUsage float64
	memUsage float64
}

func mergeStats(old, current []ProcStat) []MergedProcStat {
	pidToStat := make(map[int]ProcStat)
	for _, oldStat := range old {
		pidToStat[oldStat.pid] = oldStat
	}

	mergedStat := make([]MergedProcStat, 0, len(current))

	for _, newStat := range current {
		oldStat, ok := pidToStat[newStat.pid]
		if !ok {
			continue
		}
		uptime := newStat.now.Nanosecond()/int(time.Millisecond) - oldStat.now.Nanosecond()/int(time.Millisecond)
		cpuUsage := float64(newStat.cpu.Total-oldStat.cpu.Total) / float64(uptime)
		memUsage := float64(newStat.memory.Resident)
		mergedStat = append(mergedStat, MergedProcStat{newStat.pid, newStat.state.Name, cpuUsage, memUsage})
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
