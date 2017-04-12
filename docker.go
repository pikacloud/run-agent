package main

import (
	"context"
	"encoding/json"
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

// AgentContainer describes docker container
type AgentContainer struct {
	ID   string `json:"cid"`
	Data string `json:"data"`
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
		} else {
			log.Println("Sync docker info OK")
		}
		time.Sleep(3 * time.Second)
	}
}

func (agent *Agent) syncDockerContainers() {
	cli, err := docker_client.NewEnvClient()
	if err != nil {
		panic(err)
	}
	containersListOpts := docker_types.ContainerListOptions{
		All: true,
	}
	for {
		uri := fmt.Sprintf("run/agents/%s/docker/containers/", agent.ID)
		containers, _ := cli.ContainerList(context.Background(), containersListOpts)
		var containersCreateList []AgentContainer
		for _, container := range containers {
			data, err := json.Marshal(container)
			if err != nil {
				log.Printf("Cannot decode %v", container)
				continue
			}
			containersCreateList = append(containersCreateList, AgentContainer{ID: container.ID, Data: string(data)})
		}
		status, err := agent.Client.Post(uri, containersCreateList, nil)
		if err != nil {
			log.Println(err.Error())
		}
		if status != 200 {
			log.Printf("Failed to push docker containers: %d", status)
		} else {
			log.Printf("Sync docker %d containers OK", len(containersCreateList))
		}
		time.Sleep(3 * time.Second)
	}
}
