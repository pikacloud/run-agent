package main

import (
	"context"
	"log"
	"os"
	"sync"

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
	apiToken := os.Getenv("PIKACLOUD_API_TOKEN")
	baseURL := os.Getenv("PIKACLOUD_AGENT_URL")
	hostname := os.Getenv("PIKACLOUD_AGENT_HOSTNAME")
	labels := os.Getenv("PIKACLOUD_AGENT_LABELS")
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
	defer wg.Wait()
	wg.Add(1)
	go agent.infiniteSyncDockerInfo()
	wg.Add(1)
	go agent.infinitePing()
	wg.Add(1)
	go agent.listenDockerEvents()
	wg.Add(1)
	go agent.infinitePullTasks()
}
