package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

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

// Agent describes the agent
type Agent struct {
	Client       *Client
	DockerClient *docker_client.Client
	ID           string   `json:"aid"`
	PingURL      string   `json:"ping_url"`
	TTL          int      `json:"ttl"`
	Hostname     string   `json:"hostname"`
	Labels       []string `json:"labels"`
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

// TaskACKStep represent a task step ACK
type TaskACKStep struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// TaskACK reprensent a tack ACK
type TaskACK struct {
	TaskACKStep []*TaskACKStep `json:"results"`
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

func pluralize(n int) string {
	if n == 0 || n > 1 {
		return "s"
	}
	return ""
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
	case "unpause":
		var unpauseOpts = DockerUnpauseOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &unpauseOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker unpause: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerUnpause(unpauseOpts.ID)
		if err != nil {
			return err
		}
		return nil
	case "pause":
		var pauseOpts = DockerPauseOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &pauseOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker pause: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerPause(pauseOpts.ID)
		if err != nil {
			return err
		}
		return nil
	case "start":
		var startOpts = DockerStartOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &startOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker unpause: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerStart(startOpts.ID)
		if err != nil {
			return err
		}
		return nil
	case "stop":
		var stopOpts = DockerStopOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &stopOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker unpause: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerStop(stopOpts.ID, 10*time.Second)
		if err != nil {
			return err
		}
		return nil
	case "remove":
		var removeOpts = DockerRemoveOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &removeOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker remove: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerRemove(removeOpts.ID, &removeOpts)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("Unknown step method %s", step.Method)
	}
}

// Do a task
func (task *Task) Do() (TaskACK, error) {
	ack := TaskACK{}
	for _, step := range task.Steps {
		ackStep := TaskACKStep{}
		err := step.Do()
		if err != nil {
			log.Printf("%s Step %s failed with config %s (%s)", step.Plugin, step.Method, step.PluginConfig, err)
			ackStep.Message = err.Error()
			ackStep.Success = false
		} else {
			ackStep.Success = true
			ackStep.Message = "OK"
		}
		ack.TaskACKStep = append(ack.TaskACKStep, &ackStep)
	}
	return ack, nil
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
			log.Printf("Cannot ping %+v", err)
			log.Println("Trying to register lost agent")
			newAgentOpts := CreateAgentOptions{
				Hostname: agent.Hostname,
				Labels:   agent.Labels,
			}
			agent.Create(&newAgentOpts)
			agent.syncDockerContainers(syncDockerContainersOptions{})
			info, err := agent.DockerClient.Info(context.Background())
			if err != nil {
				log.Printf("Cannot fetch docker info %+v", err)
			} else {
				agent.syncDockerInfo(info)
			}

		}
		time.Sleep(1 * time.Second)
	}
}

// Ping agent
func (agent *Agent) Ping() error {
	pingURI := fmt.Sprintf("run/agents/%s/ping/", agent.ID)
	opts := PingAgentOptions{
		RunningTasks: runningTasksList,
	}
	status, err := agent.Client.Post(pingURI, opts, nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("Ping to %s returns %d\n", pingURI, status)
	}
	log.Printf("Ping OK (%d running task%s)", len(runningTasksList), pluralize(len(runningTasksList)))
	return nil
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
			if task.NeedACK {
				runningTasksList = append(runningTasksList, task.ID)
			}
			ackTask, err := task.Do()
			if err != nil {
				log.Printf("Unable to do task %s: %s", task.ID, err)
			}
			log.Printf("task %s done!", task.ID)
			if task.NeedACK {
				err := agent.ackTask(&task, &ackTask)
				if err != nil {
					log.Printf("Unable to ack task %s: %s", task.ID, err)
				} else {
					log.Printf("task %s ACKed", task.ID)
				}
				deleteRunningTasks(task.ID)
			}
		}
		time.Sleep(3 * time.Second)
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

func (agent *Agent) ackTask(task *Task, taskACK *TaskACK) error {
	ackURI := fmt.Sprintf("run/agents/%s/tasks/unack/%s/", agent.ID, task.ID)
	_, err := agent.Client.Delete(ackURI, &taskACK, nil)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
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
		log.Fatal(err)
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
