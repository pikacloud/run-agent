package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"reflect"
	"time"

	docker_types "github.com/docker/docker/api/types"
	docker_types_container "github.com/docker/docker/api/types/container"
	docker_types_event "github.com/docker/docker/api/types/events"
	docker_types_network "github.com/docker/docker/api/types/network"
)

// DockerPorts describes docker ports for docker run
type DockerPorts struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
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

// DockerUnpauseOpts describes docker unpause options
type DockerUnpauseOpts struct {
	ID string `json:"id"`
}

// DockerPauseOpts describes docker pause options
type DockerPauseOpts struct {
	ID string `json:"id"`
}

// DockerStopOpts describes docker stop options
type DockerStopOpts struct {
	ID string `json:"id"`
}

// DockerStartOpts describes docker start options
type DockerStartOpts struct {
	ID string `json:"id"`
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

type syncDockerContainersOptions struct {
	ContainersID []string
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

func (agent *Agent) dockerUnpause(containerID string) error {
	ctx := context.Background()
	if err := agent.DockerClient.ContainerUnpause(ctx, containerID); err != nil {
		return err
	}
	log.Printf("Container %s unpaused", containerID)
	return nil
}

func (agent *Agent) dockerPause(containerID string) error {
	ctx := context.Background()
	if err := agent.DockerClient.ContainerPause(ctx, containerID); err != nil {
		return err
	}
	log.Printf("Container %s paused", containerID)
	return nil
}

func (agent *Agent) dockerStop(containerID string, timeout time.Duration) error {
	ctx := context.Background()
	if err := agent.DockerClient.ContainerStop(ctx, containerID, &timeout); err != nil {
		return err
	}
	log.Printf("Container %s stopped", containerID)
	return nil
}

func (agent *Agent) syncDockerInfo() {
	dockerInfoState := docker_types.Info{}
	for {
		uri := fmt.Sprintf("run/agents/%s/docker/info/", agent.ID)
		info, _ := agent.DockerClient.Info(context.Background())
		pingInfo := AgentDockerInfo{
			Info: info,
		}
		// compare
		dockerInfoState.SystemTime = ""
		info.SystemTime = ""
		if !reflect.DeepEqual(dockerInfoState, info) {
			status, err := agent.Client.Put(uri, pingInfo, nil)
			if err != nil {
				log.Println(err.Error())
			}
			if status != 200 {
				log.Printf("Failed to push docker info: %d", status)
			} else {
				dockerInfoState = info
				log.Println("Sync docker info OK")
			}
		}
		time.Sleep(3 * time.Second)
	}
}

func (agent *Agent) syncDockerContainers(opts syncDockerContainersOptions) error {
	containersListOpts := docker_types.ContainerListOptions{
		All: true,
	}
	var containersCreateList []AgentContainer
	uri := fmt.Sprintf("run/agents/%s/docker/containers/", agent.ID)
	containers, err := agent.DockerClient.ContainerList(context.Background(), containersListOpts)
	if err != nil {
		return err
	}
	for _, container := range containers {
		data, err := json.Marshal(container)
		if err != nil {
			log.Printf("Cannot decode %v", container)
			continue
		}
		if len(opts.ContainersID) > 0 {
			skip := true
			for _, c := range opts.ContainersID {
				if c == container.ID {
					skip = false
					break
				}
			}
			if skip {
				continue
			}
		}
		inspect, err := agent.DockerClient.ContainerInspect(context.Background(), container.ID)
		if err != nil {
			log.Printf("Cannot inspect container %v", err)
			continue
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
	if len(containersCreateList) > 0 {
		status, err := agent.Client.Post(uri, containersCreateList, nil)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("Failed to push docker containers: %d", status)
		}
		log.Printf("Sync docker %d containers of %d OK", len(containersCreateList), len(containers))
	}

	return nil

}

func (agent *Agent) unsyncDockerContainer(containerID string) error {
	deleteContainerURI := fmt.Sprintf("run/agents/%s/docker/containers/%s/", agent.ID, containerID)
	_, err := agent.Client.Delete(deleteContainerURI, nil, nil)
	if err != nil {
		return err
	}
	log.Printf("Container %s garbage collected", containerID)
	return nil
}

func (agent *Agent) parseContainerEvent(msg docker_types_event.Message) error {
	containerID := msg.ID
	switch msg.Action {
	case "destroy":
		err := agent.unsyncDockerContainer(containerID)
		if err != nil {
			return err
		}
	default:

		opts := syncDockerContainersOptions{
			ContainersID: []string{containerID},
		}
		err := agent.syncDockerContainers(opts)
		if err != nil {
			return err
		}
	}
	return nil
}

func (agent *Agent) parseDockerEvent(msg docker_types_event.Message) error {
	switch msg.Type {
	case "container":
		err := agent.parseContainerEvent(msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (agent *Agent) listenDockerEvents() error {
	events, errs := agent.DockerClient.Events(context.Background(), docker_types.EventsOptions{})
	for {
		select {
		case dMsg := <-events:
			err := agent.parseDockerEvent(dMsg)
			if err != nil {
				log.Println(err)
			}
		case dErr := <-errs:
			if dErr != nil {
				fmt.Println(dErr)
			}
		}
	}
}

// Run docker container
func (agent *Agent) Run() {

}
