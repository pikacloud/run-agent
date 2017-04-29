package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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
	Env        []string       `json:"env"`
}

// DockerPingOpts describes the structure to ping docker containers in pikacloud API
type DockerPingOpts struct {
	Containers []string `json:"containers_id"`
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
	ID        string `json:"cid"`
	Container string `json:"container"`
	Config    string `json:"config"`
}

func (agent *Agent) dockerPull(opts *DockerPullOpts) error {
	log.Printf("Pulling %s", opts.Image)
	ctx := context.Background()
	pullOpts := docker_types.ImagePullOptions{}
	out, err := agent.DockerClient.ImagePull(ctx, opts.Image, pullOpts)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err = io.Copy(ioutil.Discard, out); err != nil {
		return err
	}
	log.Printf("New image pulled %s", opts.Image)
	return nil
}

func (agent *Agent) dockerCreate(opts *DockerCreateOpts) (*docker_types_container.ContainerCreateCreatedBody, error) {
	ctx := context.Background()
	config := &docker_types_container.Config{
		Image: opts.Image,
		Env:   opts.Env,
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

func (agent *Agent) syncDockerContainers(containersStore *[]docker_types.Container) error {
	containersListOpts := docker_types.ContainerListOptions{
		All: true,
	}
	uri := fmt.Sprintf("run/agents/%s/docker/containers/", agent.ID)
	containerPingURI := fmt.Sprintf("run/agents/%s/docker/containers/?ping", agent.ID)
	var containers []docker_types.Container
	containers, _ = agent.DockerClient.ContainerList(context.Background(), containersListOpts)
	// garbase dead containers
	for _, pastContainer := range *containersStore {
		isAbsent := true
		for _, nowContainer := range containers {
			if nowContainer.ID == pastContainer.ID {
				isAbsent = false
			}
		}
		if isAbsent {
			deleteContainerURI := fmt.Sprintf("run/agents/%s/docker/containers/%s/", agent.ID, pastContainer.ID)
			err := agent.Client.Delete(deleteContainerURI, nil)
			if err != nil {
				log.Printf("Garbage collector failing for container %s, %v", pastContainer.ID, err)
			} else {
				log.Printf("Container %s garbage collected", pastContainer.ID)
			}
		}
	}

	var containersCreateList []AgentContainer
	var containersPingList []AgentContainer
	for _, container := range containers {
		noChanges := false
		for _, pastContainer := range *containersStore {
			if container.ID == pastContainer.ID {
				if pastContainer.State == container.State {
					noChanges = true
				} else {
					log.Printf("Sync container %s", container.ID)
				}
			}
		}
		if noChanges {
			containersPingList = append(containersPingList,
				AgentContainer{
					ID: container.ID,
				})
			continue
		}
		data, err := json.Marshal(container)
		if err != nil {
			log.Printf("Cannot decode %v", container)
			continue
		}
		inspect, err := agent.DockerClient.ContainerInspect(context.Background(), container.ID)
		if err != nil {
			log.Printf("Cannot inspect container %v", err)
		}
		inspectConfig, err := json.Marshal(inspect)
		if err != nil {
			log.Printf("Cannot decode %v", inspect)
			continue
		}
		containersCreateList = append(containersCreateList,
			AgentContainer{
				ID:        container.ID,
				Container: string(data),
				Config:    string(inspectConfig),
			})
	}
	*containersStore = containers
	if len(containersPingList) > 0 {
		status, err := agent.Client.Post(containerPingURI, containersPingList, nil)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("Failed to ping docker containers: %d", status)
		}
		log.Printf("Pinging %d docker containers", len(containersPingList))
	}
	if len(containersCreateList) > 0 {
		status, err := agent.Client.Post(uri, containersCreateList, nil)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("Failed to push docker containers: %d", status)
		}
		log.Printf("Sync docker %d containers OK (%d new)", len(containers), len(containersCreateList))
	}

	return nil
}

// Run docker container
func (agent *Agent) Run() {

}
