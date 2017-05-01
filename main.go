package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	docker_types "github.com/docker/docker/api/types"
	docker_client "github.com/docker/docker/client"
)

const (
	// DefaultBaseURL is the API root URL
	defaultBaseURL = "https://pikacloud.com/api/"
	apiVersion     = "v1"
)

var (
	agent *Agent
)

// Agent describes the agent
type Agent struct {
	Client       *Client
	DockerClient *docker_client.Client
	ID           string `json:"aid"`
	PingURL      string `json:"ping_url"`
	TTL          int    `json:"ttl"`
	Hostname     string `json:"hostname"`
}

// CreateAgentOptions represents the agent Create() options
type CreateAgentOptions struct {
	Hostname string `json:"hostname"`
}

// PluginConfig describes a plugin option
type PluginConfig struct {
}

// TaskStep describes a step of a task
type TaskStep struct {
	Plugin            string          `json:"plugin"`
	PluginConfig      json.RawMessage `json:"plugin_options"`
	Method            string          `json:"method"`
	ExitOnFailure     bool            `json:"exit_on_failure"`
	WaitForCompletion bool            `json:"wait_for_completion"`
}

// Task descibres a task
type Task struct {
	ID      string      `json:"tid"`
	Steps   []*TaskStep `json:"payload"`
	NeedACK bool        `json:"need_ack"`
}

// Do a step
func (step *TaskStep) Do() error {
	switch step.Plugin {
	case "docker":
		return step.Docker()
	default:
		return fmt.Errorf("Unknown step plugin %s", step.Plugin)
	}
}

// Docker runs a docker step
func (step *TaskStep) Docker() error {
	switch step.Method {
	case "run":
		var createOpts = DockerCreateOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &createOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker container run: %s (%v)", err, step.PluginConfig)
		}
		pullOpts := &DockerPullOpts{
			Image: createOpts.Image,
		}
		err = agent.dockerPull(pullOpts)
		if err != nil {
			return err
		}
		containerCreated, err := agent.dockerCreate(&createOpts)
		if err != nil {
			return err
		}
		if err := agent.dockerStart(containerCreated.ID); err != nil {
			return err
		}
		agent.syncDockerContainers(&containers)
		return nil
	case "pull":
		var pullOpts = DockerPullOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &pullOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker pull: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerPull(&pullOpts)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("Unknown step method %s", step.Method)
	}
}

// Do a task
func (task *Task) Do() error {
	for _, step := range task.Steps {
		err := step.Do()
		if err != nil {
			log.Printf("%s Step %s failed with config %s (%s)", step.Plugin, step.Method, step.PluginConfig, err)
		}
	}
	return nil
}

// Create an agent
func (agent *Agent) Create(opt *CreateAgentOptions) error {
	status, err := agent.Client.Post("run/agents/", opt, &agent)
	if err != nil {
		return fmt.Errorf(err.Error())
	}
	if status != 200 {
		return fmt.Errorf("Failed to create agent http code: %d", status)
	}
	log.Printf("Agent %s registered with hostname %s\n", agent.ID, agent.Hostname)
	return nil
}

func (agent *Agent) infinitePing() {
	for {
		err := agent.Ping()
		if err != nil {
			log.Println(err)
		}
		time.Sleep(3 * time.Second)
	}
}

// Ping agent
func (agent *Agent) Ping() error {
	pingURI := fmt.Sprintf("run/agents/%s/ping/", agent.ID)
	// for {
	status, err := agent.Client.Post(pingURI, nil, nil)
	if err != nil {
		log.Println("Trying to register lost agent")
		newAgentOpts := CreateAgentOptions{
			Hostname: agent.Hostname,
		}
		return agent.Create(&newAgentOpts)
	}
	if status != 200 {
		return fmt.Errorf("Ping to %s returns %d\n", pingURI, status)
	}
	log.Println("Ping OK")
	return nil
}

var containers []docker_types.Container

func (agent *Agent) infiniteSyncDockerContainers() {

	for {
		log.Printf("Sync seen previously %d containers", len(containers))
		err := agent.syncDockerContainers(&containers)
		if err != nil {
			log.Println(err)
		}
		time.Sleep(3 * time.Second)
	}
}

func (agent *Agent) infinitePullTasks() {
	for {
		tasks, err := agent.pullTasks()
		if err != nil {
			log.Println(err)
		}
		if len(tasks) > 0 {
			log.Printf("Got %d new tasks", len(tasks))
		}
		for _, task := range tasks {
			err := task.Do()
			if err != nil {
				log.Printf("Unable to do task %s: %s", task.ID, err)
			}
			log.Printf("task %s done!", task.ID)
			if task.NeedACK {
				err := agent.ackTask(task)
				if err != nil {
					log.Printf("Unable to ack task %s: %s", task.ID, err)
				}
				log.Printf("task %s ACKed", task.ID)

			}
		}
		time.Sleep(1 * time.Second)
	}
}

func (agent *Agent) pullTasks() ([]Task, error) {
	tasksURI := fmt.Sprintf("run/agents/%s/tasks/ready/?requeue=false&size=50", agent.ID)
	tasks := []Task{}
	err := agent.Client.Get(tasksURI, &tasks)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return tasks, nil
}

func (agent *Agent) ackTask(t Task) error {
	ackURI := fmt.Sprintf("run/agents/%s/tasks/unack/%s/", agent.ID, t.ID)
	err := agent.Client.Delete(ackURI, nil)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func main() {
	apiToken := os.Getenv("PIKACLOUD_API_TOKEN")
	baseURL := os.Getenv("PIKACLOUD_AGENT_URL")
	if apiToken == "" {
		log.Fatalln("PIKACLOUD_API_TOKEN is empty")
	}
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("Unable to retrieve agent hostname: %v", err)
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
	err = agent.Create(&newAgentOpts)
	if err != nil {
		log.Fatal(err)
	}
	wg := sync.WaitGroup{}
	defer wg.Wait()
	wg.Add(1)
	go agent.infinitePing()
	wg.Add(1)
	go agent.syncDockerInfo()
	wg.Add(1)
	go agent.infiniteSyncDockerContainers()
	wg.Add(1)
	go agent.infinitePullTasks()

}
