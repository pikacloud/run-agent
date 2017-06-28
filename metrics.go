package main

import (
	"fmt"
	"log"
	"time"

	"github.com/shirou/gopsutil/cpu"
)

//Metrics represents a basic struct with systems stats
type Metrics struct {
	CpuStats []cpu.TimesStat `json:"cpustats"`
}

func getCPUINfo() ([]cpu.TimesStat, error) {
	cpustats, err := cpu.Times(true)
	if err != nil {
		return nil, fmt.Errorf("Error getting cpu information: %v", err)
	}
	return cpustats, nil
	//for _, _cpu := range mycpu {
	//	log.Printf("CPU: %s | User: %f | System: %f | Idle: %f | Nice: %f | Iowait: %f | Irq: %f | Softirq: %f | Steal: %f | Guest: %f | GuestNice: %f | Stolen: %f", _cpu.CPU, _cpu.User, _cpu.System, _cpu.Idle, _cpu.Nice, _cpu.Iowait, _cpu.Irq, _cpu.Softirq, _cpu.Steal, _cpu.Guest, _cpu.GuestNice, _cpu.Stolen)
	//}
	//return "toto"
}

func (agent *Agent) basicMetrics() {
	for {
		c, err := getCPUINfo()
		if err != nil {
			log.Println(err)
		}
		metrics.CpuStats = c
		//log.Println(c)
		time.Sleep(3 * time.Second)
	}
}
