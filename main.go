package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pikacloud/gopikacloud"
)

const (
	// DefaultBaseURL is the API root URL
	defaultBaseURL = "https://pikacloud.com/api/"
	apiVersion     = "v1"
)

var (
	agent             *Agent
	runningTasksList  map[string]*Task
	metrics           *Metrics
	cpuprofile        = flag.String("cpuprofile", "", "write cpu profile to file")
	showVersion       = flag.Bool("version", false, "show version")
	showLatestVersion = flag.Bool("latest", false, "show latest version available")
	autoMode          = flag.Bool("auto", false, "run with self updating mode activated")
	updaterMode       = flag.Bool("updater", false, "self update binary if necessary")
	wsURL             = "wss://ws.pikacloud.com"
	isXhyve           = false
	xhyveTTY          = "~/Library/Containers/com.docker.docker/Data/com.docker.driver.amd64-linux/tty"
	version           string
	gitRef            string
	lock              = sync.RWMutex{}
	pikacloudClient   *gopikacloud.Client
	logger            = logrus.New()
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

func shutdown(exitCode int) {
	logger.Println("Shutting down run-agent...")
	os.Exit(exitCode)
}

func execAgent() (*exec.Cmd, error) {
	cmd := exec.Command("./run-agent", "-updater")
	cmd.Env = os.Environ()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Fatalln(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		logger.Fatalln(err)
	}
	errStart := cmd.Start()
	if errStart != nil {
		logger.Fatalln(errStart)
	}
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	return cmd, nil
}

func main() {
	flag.Parse()
	loglevel := os.Getenv("LOG_LEVEL")
	switch strings.ToUpper(loglevel) {
	case "DEBUG":
		logger.SetLevel(logrus.DebugLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}
	if *autoMode {
		for {
			logger.Info("Executing run-agent")
			cmd, err := execAgent()
			if err != nil {
				logger.Errorf("Cannot execute agent: %s", err)
				time.Sleep(3 * time.Second)
			}
			logger.Info("Run-agent is running")
			errWait := cmd.Wait()
			if errWait != nil {
				logger.Errorf("Agent exited: %s", errWait)
			}
			time.Sleep(3 * time.Second)
		}
	}
	runningTasksList = make(map[string]*Task)
	metrics = &Metrics{}
	killchan := make(chan os.Signal, 2)
	signal.Notify(killchan, syscall.SIGINT, syscall.SIGTERM)

	if version == "" {
		version = "undefined"
	}
	if *showVersion {
		fmt.Printf("Run-agent version %s (git ref: %s)", version, gitRef)
		os.Exit(0)
	}
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			logger.Fatal(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			logger.Fatalf("could not start CPU profile: %+v", err)
		}
		logger.Info("Starting CPU profiler")
		defer func() {
			logger.Info("Stopping CPU profiler")
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
		logger.Fatal("PIKACLOUD_API_TOKEN is empty")
	}
	pikacloudClient = gopikacloud.NewClient(apiToken)
	if baseURL != "" {
		pikacloudClient.BaseURL = baseURL
	}
	if hostname == "" {
		h, err := os.Hostname()
		if err != nil {
			logger.Fatalf("Unable to retrieve agent hostname: %v", err)
		}
		hostname = h
	}
	agent = NewAgent(apiToken, hostname, labels)
	if baseURL != "" {
		agent.Client.BaseURL = baseURL
	}
	if *updaterMode {
		logger.Info("Checking for run-agent updates")
		errUpdate := agent.update()
		if errUpdate != nil {
			logger.Fatalf("Unable to auto-update agent: %s", errUpdate)
		}
	}

	err := agent.Register()
	if err != nil {
		logger.Fatalf("Unable to register agent: %s", err.Error())
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
	catchedSignal := <-killchan
	logger.Debugf("Signal %d catched", catchedSignal)
	pprof.StopCPUProfile()
	shutdown(0)
}
