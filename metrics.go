package main

import (
	"fmt"
	"log"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

//Metrics represents a basic struct with systems stats
type Metrics struct {
	CpuStats []cpu.TimesStat        `json:"cpustats"`
	MemStats *mem.VirtualMemoryStat `json:"memstats"`
}

func getRAMInfo() (*mem.VirtualMemoryStat, error) {
	memstats, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("Error getting memory information: %v", err)
	}
	return memstats, nil
}

func getCPUInfo() ([]cpu.TimesStat, error) {
	cpustats, err := cpu.Times(true)
	if err != nil {
		return nil, fmt.Errorf("Error getting cpu information: %v", err)
	}
	return cpustats, nil
}

func (agent *Agent) basicMetrics() {
	for {
		c, err := getCPUInfo()
		if err != nil {
			log.Println(err)
		}
		m, err := getRAMInfo()
		if err != nil {
			log.Println(err)
		}
		metrics.CpuStats = c
		metrics.MemStats = m
		time.Sleep(3 * time.Second)
	}
}
