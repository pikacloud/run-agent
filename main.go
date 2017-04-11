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

// Create an agent
func (agent *Agent) Create(opt *CreateAgentOptions) error {
	status, err := agent.Client.Post("run/agents/", opt, &agent)
	if err != nil {
		return fmt.Errorf(err.Error())
	}
	if status != 200 {
		return fmt.Errorf("Failed to create agent http code: %d", status)
	} else {
		log.Printf("Agent %s registered with hostname %s\n", agent.ID, agent.Hostname)
	}
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

}
