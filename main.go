package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"sync"
	"syscall"

	docker_client "github.com/moby/moby/client"
)

const (
	// DefaultBaseURL is the API root URL
	defaultBaseURL = "https://pikacloud.com/api/"
	apiVersion     = "v1"
)

var (
	agent            *Agent
	runningTasksList []string
	cpuprofile       = flag.String("cpuprofile", "", "write cpu profile to file")
	wsURL            = "ws.pikacloud.com"
	isXhyve          = false
	xhyveTTY         = "~/Library/Containers/com.docker.docker/Data/com.docker.driver.amd64-linux/tty"
)

func deleteRunningTasks(tid string) {
	for idx, t := range runningTasksList {
		if t == tid {
			runningTasksList = runningTasksList[:idx+copy(runningTasksList[idx:], runningTasksList[idx+1:])]
		}
	}
}

// PluginConfig describes a plugin option
type PluginConfig struct {
}

func pluralize(n int) string {
	if n == 0 || n > 1 {
		return "s"
	}
	return ""
}

func main() {
	killchan := make(chan os.Signal, 2)
	signal.Notify(killchan, syscall.SIGINT, syscall.SIGTERM)
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		log.Println("Starting CPU profiler")
		defer func() {
			log.Println("Stopping CPU profiler")
			pprof.StopCPUProfile()
		}()
	}
	runningTerminalsList = make(map[*DockerTerminalOpts]bool)
	apiToken := os.Getenv("PIKACLOUD_API_TOKEN")
	baseURL := os.Getenv("PIKACLOUD_AGENT_URL")
	hostname := os.Getenv("PIKACLOUD_AGENT_HOSTNAME")
	labels := os.Getenv("PIKACLOUD_AGENT_LABELS")
	wsURLenv := os.Getenv("PIKACLOUD_WS_URL")
	isXhyvEnv := os.Getenv("PIKACLOUD_XHYVE")
	xhyveTTYEnv := os.Getenv("PIKACLOUD_XHYVE_TTY")
	if xhyveTTYEnv != "" {
		xhyveTTY = xhyveTTYEnv
	}
	if isXhyvEnv != "" {
		isXhyve = true
	}
	if wsURLenv != "" {
		wsURL = wsURLenv
	}

	if apiToken == "" {
		log.Fatalln("PIKACLOUD_API_TOKEN is empty")
	}
	if hostname == "" {
		h, err := os.Hostname()
		if err != nil {
			log.Fatalf("Unable to retrieve agent hostname: %v", err)
		}
		hostname = h
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	c := NewClient(apiToken)
	c.BaseURL = baseURL
	dockerClient, err := docker_client.NewEnvClient()
	if err != nil {
		panic(err)
	}
	_, err = dockerClient.ServerVersion(context.Background())
	if err != nil {
		panic(err)
	}

	_agent := Agent{
		Client:       c,
		Hostname:     hostname,
		DockerClient: dockerClient,
	}
	agent = &_agent
	newAgentOpts := CreateAgentOptions{
		Hostname: hostname,
	}
	if labels != "" {
		newAgentOpts.Labels = makeLabels(labels)
		_agent.Labels = makeLabels(labels)
	}

	err = agent.Create(&newAgentOpts)
	if err != nil {
		log.Fatalf("Unable to register agent: %s", err.Error())
	}
	agent.syncDockerContainers(syncDockerContainersOptions{})
	wg := sync.WaitGroup{}
	defer func() {
		wg.Wait()
		pprof.StopCPUProfile()
	}()
	wg.Add(1)
	go agent.infiniteSyncDockerInfo()
	wg.Add(1)
	go agent.infinitePing()
	wg.Add(1)
	go agent.listenDockerEvents()
	wg.Add(1)
	go agent.infinitePullTasks()
	<-killchan
	pprof.StopCPUProfile()
	os.Exit(0)
}
