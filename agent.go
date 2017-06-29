package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	docker_client "github.com/moby/moby/client"
)

// CreateAgentOptions represents the agent Create() options
type CreateAgentOptions struct {
	Hostname  string   `json:"hostname"`
	Labels    []string `json:"labels,omitempty"`
	Localtime int      `json:"localtime"`
	Version   string   `json:"version"`
}

// PingAgentOptions represents the agent Ping() options
type PingAgentOptions struct {
	Metrics          *Metrics `json:"metrics,omitempty"`
	RunningTasks     []string `json:"running_tasks,omitempty"`
	RunningTerminals []string `json:"running_terminals,omitempty"`
	Localtime        int      `json:"localtime"`
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
	Localtime    int      `json:"localtime"`
}

func localtime() int {
	return int(time.Now().Unix())
}

func makeLabels(labels string) []string {
	labelsList := strings.Split(labels, ",")
	ret := make([]string, len(labelsList))
	for idx, l := range labelsList {
		if l == "" {
			return nil
		}
		l = strings.TrimSpace(l)
		l = strings.Join(strings.Fields(l), "_")
		ret[idx] = l
	}
	return ret
}

// NewAgent create a new agent
func NewAgent(apiToken string, hostname string, labels []string) *Agent {
	return &Agent{
		Hostname:     hostname,
		Client:       NewClient(apiToken),
		DockerClient: NewDockerClient(),
		Labels:       labels,
	}
}

// Register an agent
func (agent *Agent) Register() error {
	opt := CreateAgentOptions{
		Hostname:  agent.Hostname,
		Localtime: localtime(),
		Version:   version,
	}
	if len(agent.Labels) > 0 {
		opt.Labels = agent.Labels
	}
	j, _ := json.Marshal(opt)
	fmt.Println(agent.Labels, len(agent.Labels), string(j))
	status, err := agent.Client.Post("run/agents/", opt, &agent)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("Failed to create agent http code: %d", status)
	}
	log.Printf("Agent %s registered with hostname %s (agent version %s)\n", agent.ID, agent.Hostname, version)
	return nil
}

// AgentLatestVersion represents the latest agent version available
type AgentLatestVersion struct {
	Version string `json:"version"`
}

// lastVersion checks the latest run-agent available
func (agent *Agent) latestVersion() (string, error) {
	version := AgentLatestVersion{}
	err := agent.Client.Get("run/agent-version/", &version)
	if err != nil {
		return "", err
	}
	return version.Version, nil
}

func (agent *Agent) infinitePing() {
	for {
		err := agent.Ping()
		if err != nil {
			log.Printf("Cannot ping %+v", err)
			log.Println("Trying to register lost agent")
			agent.Register()
			agent.syncDockerContainers(syncDockerContainersOptions{})
			info, err := agent.DockerClient.Info(context.Background())
			if err != nil {
				log.Printf("Cannot fetch docker info %+v", err)
			} else {
				agent.syncDockerInfo(info)
			}

		}
		time.Sleep(3 * time.Second)
	}
}

// Ping agent
func (agent *Agent) Ping() error {
	pingURI := fmt.Sprintf("run/agents/%s/ping/", agent.ID)
	opts := PingAgentOptions{
		Metrics:      metrics,
		RunningTasks: runningTasksList,
		Localtime:    localtime(),
	}
	for t := range runningTerminalsList {
		opts.RunningTerminals = append(opts.RunningTerminals, t.Task.ID)
	}
	status, err := agent.Client.Post(pingURI, opts, nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("ping to %s returns %d codes", pingURI, status)
	}
	log.Printf("Ping OK (%d running task%s, %d running terminal%s)", len(opts.RunningTasks), pluralize(len(opts.RunningTasks)), len(opts.RunningTerminals), pluralize(len(opts.RunningTerminals)))
	return nil
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
			err := task.Do()
			if err != nil {
				log.Printf("Unable to do task %s: %s", task.ID, err)
			}
			log.Printf("task %s done!", task.ID)
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
