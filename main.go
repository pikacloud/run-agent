package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"sync"
	"syscall"

	"github.com/pikacloud/gopikacloud"
)

const (
	// DefaultBaseURL is the API root URL
	defaultBaseURL = "https://pikacloud.com/api/"
	apiVersion     = "v1"
)

var (
	agent             *Agent
	runningTasksList  []string
	metrics           *Metrics
	cpuprofile        = flag.String("cpuprofile", "", "write cpu profile to file")
	showVersion       = flag.Bool("version", false, "show version")
	showLatestVersion = flag.Bool("latest", false, "show latest version available")
	wsURL             = "ws.pikacloud.com"
	isXhyve           = false
	xhyveTTY          = "~/Library/Containers/com.docker.docker/Data/com.docker.driver.amd64-linux/tty"
	version           string
	lock              = sync.RWMutex{}
	pikacloudClient   *gopikacloud.Client
)

// PluginConfig describes a plugin option
type PluginConfig struct {
}

func pluralize(n int) string {
	if n == 0 || n > 1 {
		return "s"
	}
	return ""
}

func shutdown() {
	os.Exit(0)
}

func main() {
	metrics = &Metrics{}
	killchan := make(chan os.Signal, 2)
	signal.Notify(killchan, syscall.SIGINT, syscall.SIGTERM)
	flag.Parse()
	if *showVersion {
		fmt.Printf("Run-agent version %s", version)
		os.Exit(0)
	}
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
	envLabels := os.Getenv("PIKACLOUD_AGENT_LABELS")
	wsURLenv := os.Getenv("PIKACLOUD_WS_URL")
	isXhyvEnv := os.Getenv("PIKACLOUD_XHYVE")
	xhyveTTYEnv := os.Getenv("PIKACLOUD_XHYVE_TTY")
	labels := makeLabels(envLabels)
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
	pikacloudClient = gopikacloud.NewClient(apiToken)
	if baseURL != "" {
		pikacloudClient.BaseURL = baseURL
	}
	if hostname == "" {
		h, err := os.Hostname()
		if err != nil {
			log.Fatalf("Unable to retrieve agent hostname: %v", err)
		}
		hostname = h
	}
	agent = NewAgent(apiToken, hostname, labels)
	if baseURL != "" {
		agent.Client.BaseURL = baseURL
	}
	err := agent.Register()
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
	wg.Add(1)
	go agent.basicMetrics()
	<-killchan
	pprof.StopCPUProfile()
	os.Exit(0)
}
