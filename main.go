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
	Client  *Client
	ID      string `json:"aid"`
	PingURL string `json:"ping_url"`
	TTL     int    `json:"ttl"`
}

// Create an agent
func (agent *Agent) Create() error {
	status, err := agent.Client.Post("run/agents/", nil, &agent)
	if err != nil {
		log.Fatalf(err.Error())
	}
	if status != 200 {
		log.Fatalf("Failed to create agent http code: %d", status)
	}
	log.Printf("Agent %s registered\n", agent.ID)
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
		return err
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
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	c := NewClient(apiToken)
	c.BaseURL = baseURL

	agent := Agent{
		Client: c,
	}
	agent.Create()
	wg := sync.WaitGroup{}
	defer wg.Wait()

	wg.Add(1)
	agent.infinitePing()

}
