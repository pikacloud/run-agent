package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the API root URL
	defaultBaseURL = "https://pikacloud.com/api/"
	apiVersion     = "v1"
)

// Agent describes the agent
type Agent struct {
	Client   *Client
	ID       string `json:"aid"`
	PingURL  string `json:"ping_url"`
	TTL      int    `json:"ttl"`
	Hostname string `json:"hostname"`
}

// CreateAgentOptions represents the agent Create() options
type CreateAgentOptions struct {
	Hostname string `json:"hostname"`
}

// Task descibres a task
type Task struct {
	ID      string `json:"tid"`
	Payload string `json:"payload"`
	NeedACK bool   `json:"need_ack"`
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
		log.Println("Ping OK")
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

	agent := Agent{
		Client:   c,
		Hostname: hostname,
	}
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
	go agent.syncDockerContainers()
	wg.Add(1)
	go agent.infinitePullTasks()

}
