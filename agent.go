package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	docker_client "github.com/moby/moby/client"
)

// CreateAgentOptions represents the agent Create() options
type CreateAgentOptions struct {
	Hostname string   `json:"hostname"`
	Labels   []string `json:"labels,omitempty"`
}

// PingAgentOptions represents the agent Ping() options
type PingAgentOptions struct {
	RunningTasks []string `json:"running_tasks,omitempty"`
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

func makeLabels(labels string) []string {
	return strings.Split(labels, ",")
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
		time.Sleep(3 * time.Second)
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
