package main

import (
	"errors"
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

// CreateAgent creates an agent
func (client *Client) CreateAgent() (Agent, error) {
	res := Agent{}
	status, err := client.Post("run/agents/", nil, &res)
	if err != nil {
		return Agent{}, err
	}
	if status == 200 {
		return res, nil
	}
	return res, errors.New("Failed to create agent")
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

// Ping agent
func (agent *Agent) Ping() {
	pingURI := fmt.Sprintf("run/agents/%s/ping/", agent.ID)
	for {
		status, err := agent.Client.Post(pingURI, nil, nil)
		if err != nil {
			log.Println(err)
		}

		if status == 200 {
			fmt.Println("Ping OK")
		} else {
			fmt.Printf("Ping to %s returns %d\n", pingURI, status)
		}
		// }()
		time.Sleep(1000 * time.Millisecond)
	}
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
	// agent, err := c.CreateAgent()
	// if err != nil {
	// 	log.Fatalln(err.Error())
	// }
	agent := Agent{
		Client: c,
	}
	agent.Create()
	wg := sync.WaitGroup{}
	defer wg.Wait()

	wg.Add(1)
	go func() {

		agent.Ping()
	}()
}
