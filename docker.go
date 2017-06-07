package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"reflect"
	"strconv"
	"time"

	docker_types "github.com/docker/docker/api/types"
	docker_types_container "github.com/docker/docker/api/types/container"
	docker_types_event "github.com/docker/docker/api/types/events"
	docker_types_network "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	docker_nat "github.com/docker/go-connections/nat"
)

// DockerPorts describes docker ports for docker run
type DockerPorts struct {
	HostIP        string `json:"host_ip"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// DockerCreateOpts describes docker run options
type DockerCreateOpts struct {
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	Remove     bool              `json:"rm"`
	Ports      []*DockerPorts    `json:"ports"`
	PublishAll bool              `json:"publish_all"`
	Command    string            `json:"command"`
	Entrypoint string            `json:"entrypoint"`
	Env        []string          `json:"env"`
	Binds      []string          `json:"binds"`
	User       string            `json:"user"`
	WorkingDir string            `json:"working_dir"`
	Labels     map[string]string `json:"labels"`
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

// DockerRestartOpts describes docker restart options
type DockerRestartOpts struct {
	ID string `json:"id"`
}

// DockerRemoveOpts describes docker remove options
type DockerRemoveOpts struct {
	ID            string `json:"id"`
	Force         bool   `json:"force"`
	RemoveLinks   bool   `json:"remove_links"`
	RemoveVolumes bool   `json:"remove_volumes"`
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
		Image:  opts.Image,
		Env:    opts.Env,
		Labels: opts.Labels,
	}
	if opts.User != "" {
		config.User = opts.User
	}
	if opts.WorkingDir != "" {
		config.WorkingDir = opts.WorkingDir
	}
	if opts.Entrypoint != "" {
		config.Entrypoint = strslice.StrSlice{opts.Entrypoint}
	}
	if opts.Command != "" {
		config.Cmd = strslice.StrSlice{opts.Command}
	}
	natPortmap := docker_nat.PortMap{}
	for _, p := range opts.Ports {
		containerPortProto := docker_nat.Port(fmt.Sprintf("%d/%s", p.ContainerPort, p.Protocol))
		dockerHostConfig := docker_nat.PortBinding{
			HostIP:   p.HostIP,
			HostPort: strconv.Itoa(p.HostPort),
		}
		natPortmap[containerPortProto] = append(natPortmap[containerPortProto], dockerHostConfig)
	}
	hostConfig := &docker_types_container.HostConfig{
		Binds:           opts.Binds,
		PublishAllPorts: opts.PublishAll,
		PortBindings:    natPortmap,
	}
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

func (agent *Agent) dockerRestart(containerID string, timeout time.Duration) error {
	ctx := context.Background()
	if err := agent.DockerClient.ContainerRestart(ctx, containerID, &timeout); err != nil {
		return err
	}
	log.Printf("Container %s restarted", containerID)
	return nil
}

func (agent *Agent) dockerRemove(containerID string, opts *DockerRemoveOpts) error {
	removeOpts := docker_types.ContainerRemoveOptions{
		Force:         opts.Force,
		RemoveVolumes: opts.RemoveVolumes,
		RemoveLinks:   opts.RemoveLinks,
	}
	ctx := context.Background()
	if err := agent.DockerClient.ContainerRemove(ctx, containerID, removeOpts); err != nil {
		return err
	}
	log.Printf("Container %s remove", containerID)
	return nil
}

func (agent *Agent) infiniteSyncDockerInfo() {
	dockerInfoState := docker_types.Info{}
	for {
		info, err := agent.DockerClient.Info(context.Background())
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		// compare
		dockerInfoState.SystemTime = ""
		info.SystemTime = ""
		if !reflect.DeepEqual(dockerInfoState, info) {
			err := agent.syncDockerInfo(info)
			if err != nil {
				log.Printf("Cannot sync docker info: %+v", err)
			} else {
				dockerInfoState = info
				log.Println("Sync docker info OK")
			}
		}
		time.Sleep(3 * time.Second)
	}
}

func (agent *Agent) syncDockerInfo(info docker_types.Info) error {
	uri := fmt.Sprintf("run/agents/%s/docker/info/", agent.ID)
	pingInfo := AgentDockerInfo{
		Info: info,
	}
	status, err := agent.Client.Put(uri, pingInfo, nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("Failed to push docker info: %d", status)
	}
	return nil
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

// Docker runs a docker step
func (step *TaskStep) Docker() error {
	switch step.Method {
	case "run":
		var createOpts = DockerCreateOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &createOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker container run: %s (%v)", err, step.PluginConfig)
		}
		pullOpts := &DockerPullOpts{
			Image: createOpts.Image,
		}
		err = agent.dockerPull(pullOpts)
		if err != nil {
			return err
		}
		containerCreated, err := agent.dockerCreate(&createOpts)
		if err != nil {
			return err
		}
		if err := agent.dockerStart(containerCreated.ID); err != nil {
			return err
		}
		return nil
	case "pull":
		var pullOpts = DockerPullOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &pullOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker pull: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerPull(&pullOpts)
		if err != nil {
			return err
		}
		return nil
	case "unpause":
		var unpauseOpts = DockerUnpauseOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &unpauseOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker unpause: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerUnpause(unpauseOpts.ID)
		if err != nil {
			return err
		}
		return nil
	case "pause":
		var pauseOpts = DockerPauseOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &pauseOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker pause: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerPause(pauseOpts.ID)
		if err != nil {
			return err
		}
		return nil
	case "start":
		var startOpts = DockerStartOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &startOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker unpause: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerStart(startOpts.ID)
		if err != nil {
			return err
		}
		return nil
	case "stop":
		var stopOpts = DockerStopOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &stopOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker unpause: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerStop(stopOpts.ID, 10*time.Second)
		if err != nil {
			return err
		}
		return nil
	case "restart":
		var restartOpts = DockerRestartOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &restartOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker restart: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerRestart(restartOpts.ID, 10*time.Second)
		if err != nil {
			return err
		}
		return nil
	case "remove":
		var removeOpts = DockerRemoveOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &removeOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker remove: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerRemove(removeOpts.ID, &removeOpts)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("Unknown step method %s", step.Method)
	}
}
