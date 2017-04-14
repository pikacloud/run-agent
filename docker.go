package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	docker_types "github.com/docker/docker/api/types"
	docker_types_container "github.com/docker/docker/api/types/container"
	docker_types_network "github.com/docker/docker/api/types/network"
)

// DockerPorts describes docker ports for docker run
type DockerPorts struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// DockerStartOpts describes docker start options
type DockerStartOpts struct {
}

// DockerCreateOpts describes docker run options
type DockerCreateOpts struct {
	Name       string         `json:"name"`
	Image      string         `json:"image"`
	Remove     bool           `json:"rm"`
	Ports      []*DockerPorts `json:"ports"`
	PublishAll bool           `json:"publish_all"`
	Command    string         `json:"command"`
}

// DockerPullOpts describes docker pull options
type DockerPullOpts struct {
	Image string `json:"image"`
}

// AgentDockerInfo describes docker info
type AgentDockerInfo struct {
	Info docker_types.Info `json:"info"`
}

// AgentContainer describes docker container
type AgentContainer struct {
	ID   string `json:"cid"`
	Data string `json:"data"`
}

func (agent *Agent) dockerPull(opts *DockerPullOpts) error {
	ctx := context.Background()
	pullOpts := docker_types.ImagePullOptions{}
	out, err := agent.DockerClient.ImagePull(ctx, opts.Image, pullOpts)
	if err != nil {
		return err
	}
	defer out.Close()
	log.Printf("New image pulled %s", opts.Image)
	return nil
}

func (agent *Agent) dockerCreate(opts *DockerCreateOpts) (*docker_types_container.ContainerCreateCreatedBody, error) {
	ctx := context.Background()
	config := &docker_types_container.Config{
		Image: opts.Image,
	}
	hostConfig := &docker_types_container.HostConfig{}
	networkingConfig := &docker_types_network.NetworkingConfig{}

	container, err := agent.DockerClient.ContainerCreate(ctx, config, hostConfig, networkingConfig, opts.Name)
	if err != nil {
		return nil, err
	}
	log.Printf("New container created %s", container.ID)
	return &container, nil
}

func (agent *Agent) dockerStart(containerID string) error {
	ctx := context.Background()
	startOpts := docker_types.ContainerStartOptions{}
	if err := agent.DockerClient.ContainerStart(ctx, containerID, startOpts); err != nil {
		return err
	}
	log.Printf("New container started %s", containerID)
	return nil
}

func (agent *Agent) syncDockerInfo() {
	for {
		uri := fmt.Sprintf("run/agents/%s/docker/info/", agent.ID)
		info, _ := agent.DockerClient.Info(context.Background())
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
	containersListOpts := docker_types.ContainerListOptions{
		All: true,
	}
	for {
		uri := fmt.Sprintf("run/agents/%s/docker/containers/", agent.ID)
		containers, _ := agent.DockerClient.ContainerList(context.Background(), containersListOpts)
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

// Run docker container
func (agent *Agent) Run() {

}
