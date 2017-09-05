package main

import (
	"fmt"
	"log"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

//Metrics represents a basic struct with systems stats
type Metrics struct {
	//CPU
	CPUStats []cpu.TimesStat `json:"cpustats"`
	//Memory
	MemStats  *mem.VirtualMemoryStat `json:"memstats"`
	SwapStats *mem.SwapMemoryStat    `json:"swapstats"`
	//Disk
	IOStats        map[string]disk.IOCountersStat `json:"iostats"`
	PartitionStats []disk.PartitionStat           `json:"partitionstats"`
	UsageStats     []*disk.UsageStat              `json:"usagestats"`
	//Load Average
	LoadAverage *load.AvgStat  `json:"loadaverage"`
	MiscStats   *load.MiscStat `json:"miscstats"`
	//Network
	InterfaceStats []net.InterfaceStat  `json:"interfacestats"`
	IONetStats     []net.IOCountersStat `json:"ionetstats"`
	//Agent pikacloud
	Labels []string `json:"labels"`
}

func getNetInfo() ([]net.InterfaceStat, []net.IOCountersStat, error) {
	istats, err := net.Interfaces()
	if err != nil {
		return nil, nil, fmt.Errorf("Error getting network interfaces stats: %v", err)
	}
	ionetstats, err := net.IOCounters(true)
	if err != nil {
		return nil, nil, fmt.Errorf("Error getting network io stats for %v", err)
	}
	return istats, ionetstats, nil
}

func getLoadAverage() (*load.AvgStat, *load.MiscStat, error) {
	lastats, err := load.Avg()
	if err != nil {
		return nil, nil, fmt.Errorf("Error getting load average information: %v", err)
	}
	msstats, err := load.Misc()
	if err != nil {
		return nil, nil, fmt.Errorf("Error getting Misc information: %v", err)
	}
	return lastats, msstats, nil
}

func getDiskInfo() (map[string]disk.IOCountersStat, []disk.PartitionStat, []*disk.UsageStat, error) {
	pstats, err := disk.Partitions(false)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Error getting partition stats: %v", err)
	}
	iostats, err := disk.IOCounters()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Error getting io stats for %v", err)
	}
	var usstats []*disk.UsageStat
	for _, partition := range pstats {
		usstat, err := disk.Usage(partition.Device)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("Error getting usage stats for %s: %v", partition.Device, err)
		}
		usstats = append(usstats, usstat)
	}
	return iostats, pstats, usstats, nil
}

func getSWAPInfo() (*mem.SwapMemoryStat, error) {
	swapstats, err := mem.SwapMemory()
	if err != nil {
		return nil, fmt.Errorf("Error getting swap information: %v", err)
	}
	return swapstats, nil
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
		s, err := getSWAPInfo()
		if err != nil {
			log.Println(err)
		}
		io, ps, us, err := getDiskInfo()
		if err != nil {
			log.Println(err)
		}
		la, ms, err := getLoadAverage()
		if err != nil {
			log.Println(err)
		}
		ni, ionet, err := getNetInfo()
		if err != nil {
			log.Println(err)
		}
		metrics.CPUStats = c
		metrics.MemStats = m
		metrics.SwapStats = s
		metrics.IOStats = io
		metrics.PartitionStats = ps
		metrics.UsageStats = us
		metrics.LoadAverage = la
		metrics.MiscStats = ms
		metrics.InterfaceStats = ni
		metrics.IONetStats = ionet
		metrics.Labels = agent.Labels
		time.Sleep(3 * time.Second)
	}
}
