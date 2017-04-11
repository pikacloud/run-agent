package main

import (
	"context"
	"fmt"
	"log"
	"time"

	docker_types "github.com/docker/docker/api/types"
	docker_client "github.com/docker/docker/client"
)

// AgentDockerInfo describes docker info
type AgentDockerInfo struct {
	Info docker_types.Info `json:"info"`
}

func (agent *Agent) syncDockerInfo() {
	cli, err := docker_client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	for {
		uri := fmt.Sprintf("run/agents/%s/docker/info/", agent.ID)
		info, _ := cli.Info(context.Background())
		updateInfo := AgentDockerInfo{
			Info: info,
		}
		status, err := agent.Client.Put(uri, updateInfo, nil)
		if err != nil {
			log.Println(err.Error())
		}
		if status != 200 {
			log.Printf("Failed to push docker info: %d", status)
		}
		log.Println("Sync docker info OK")
		time.Sleep(3 * time.Second)
	}

}
