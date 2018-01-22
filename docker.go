package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	docker_types "github.com/docker/docker/api/types"
	docker_types_container "github.com/docker/docker/api/types/container"
	docker_types_event "github.com/docker/docker/api/types/events"
	docker_types_network "github.com/docker/docker/api/types/network"
	docker_types_strslice "github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/pkg/progress"
	"github.com/docker/docker/pkg/streamformatter"
	docker_nat "github.com/docker/go-connections/nat"
	"github.com/google/shlex"
	"github.com/gorilla/websocket"
	"github.com/moby/moby/builder"
	"github.com/moby/moby/builder/dockerignore"
	docker_client "github.com/moby/moby/client"
	"github.com/moby/moby/pkg/archive"
	"github.com/moby/moby/pkg/fileutils"
	"github.com/moby/moby/pkg/jsonmessage"
	"github.com/moby/moby/pkg/stringid"

	"github.com/Sirupsen/logrus"
)

const (
	defaultContainerStartWait = 3
)

var (
	trackedContainers map[string]*AgentContainer
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
	Name           string            `json:"name"`
	Remove         bool              `json:"rm"`
	Ports          []*DockerPorts    `json:"ports"`
	PublishAll     bool              `json:"publish_all"`
	Command        string            `json:"command"`
	Entrypoint     string            `json:"entrypoint"`
	Env            []string          `json:"env"`
	Binds          []string          `json:"binds"`
	User           string            `json:"user"`
	WorkingDir     string            `json:"working_dir"`
	Labels         map[string]string `json:"labels"`
	PullOpts       *DockerPullOpts   `json:"pull_opts"`
	WaitForRunning int64             `json:"wait_for_running"`
	Networks       []*subNetwork     `json:"networks"`
	DNSName        string            `json:"dns_name"`
}

// DockerPingOpts describes the structure to ping docker containers in pikacloud API
type DockerPingOpts struct {
	Containers []string `json:"containers_id"`
}

// ExternalAuthPullOpts describes auth parametes for private docker pull
type ExternalAuthPullOpts struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// DockerImageTagOpts describes docker tag options
type DockerImageTagOpts struct {
	Source string `json:"source_image"`
	Target string `json:"target_image"`
}

// DockerPullOpts describes docker pull options
type DockerPullOpts struct {
	Image                string                `json:"image"`
	AlwaysPull           bool                  `json:"always_pull"`
	ExternalAuth         *ExternalAuthPullOpts `json:"external_registry_auth"`
	InternalRegistryAuth bool                  `json:"internal_registry_auth"`
}

// DockerPushOpts describes docker push options
type DockerPushOpts DockerPullOpts

type dockerInfo struct {
	ServerVersion string `json:"server_version"`
	KernelVersion string `json:"kernel_version"`
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
	ID       string            `json:"id"`
	Networks map[string]string `json:"networks"`
}

// DockerStartOpts describes docker start options
type DockerStartOpts struct {
	ID             string        `json:"id"`
	WaitForRunning int64         `json:"wait_for_running"`
	Networks       []*subNetwork `json:"networks"`
	DNSName        string        `json:"dns_name"`
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

// DockerTerminalOpts describes docker terminal options
type DockerTerminalOpts struct {
	Cid string `json:"cid"`
	Tid string `json:"tid"`
}

// DockerBuildOpts describes docker build options
type DockerBuildOpts struct {
	Tag                               string `json:"tag"`
	Path                              string `json:"path"`
	ClearBuildDir                     bool   `json:"clear_build_dir"`
	BuildID                           string `json:"build_id"`
	RemoveIntermediateContainers      bool   `json:"remove_intermediate_containers"`
	ForceRemoveIntermediateContainers bool   `json:"force_remove_intermediate_containers"`
}

var (
	runningTerminalsList map[*DockerTerminalOpts]bool
)

type windowSize struct {
	Rows uint `json:"rows"`
	Cols uint `json:"cols"`
	X    uint16
	Y    uint16
}

// AgentDockerInfo describes docker info
type AgentDockerInfo struct {
	Info docker_types.Info `json:"info"`
}

// AgentContainerStartRetry describes the start retry status of container
type AgentContainerStartRetry struct {
	FailureCount       int64  `json:"failure_count"`
	LastError          string `json:"last_error"`
	LastErrorTimestamp int64  `json:"last_error_timestamp"`
}

// AgentContainer describes docker container
type AgentContainer struct {
	ID                       string                    `json:"cid"`
	Container                string                    `json:"container"`
	Config                   string                    `json:"config"`
	AgentContainerStartRetry *AgentContainerStartRetry `json:"start_retry"`
	Networks                 []*subNetwork             `json:"networks"`
}

type syncDockerContainersOptions struct {
	ContainersID []string
}

type containerLog struct {
	containerID string
	msg         []byte
}

func searchDockerServerAPIVersion() (string, error) {
	cmd := exec.Command("docker", "version", "-f", "{{.Server.APIVersion}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	toScan := bytes.NewReader(out)
	scanner := bufio.NewScanner(toScan)
	var serverVersion string
	for scanner.Scan() {
		serverVersion = scanner.Text()
		break
	}
	return serverVersion, nil
}

// NewDockerClient creates a new docker client
func NewDockerClient(dockerServerAPIVersion string) *docker_client.Client {
	if dockerServerAPIVersion != "" {
		os.Setenv("DOCKER_API_VERSION", dockerServerAPIVersion)
	}
	dockerClient, err := docker_client.NewEnvClient()
	if err != nil {
		logger.Fatal(err)
	}
	_, err = dockerClient.ServerVersion(context.Background())
	if err != nil {
		if dockerServerAPIVersion == "" {
			versionServer, err := searchDockerServerAPIVersion()
			if err != nil || versionServer == "" {
				logger.Fatal(err, versionServer)
			}
			return NewDockerClient(versionServer)
		}
		logger.Fatal(err)
	}
	return dockerClient
}

func (agent *Agent) trackedContainer(containerID string) (*AgentContainer, error) {
	containers, err := agent.DockerClient.ContainerList(context.Background(), docker_types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	var c docker_types.Container
	for _, container := range containers {
		if container.ID == containerID {
			c = container
			break
		}
	}
	if c.ID == "" {
		return nil, fmt.Errorf("Cannot fetch container %s", containerID)
	}
	data, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("Cannot decode %v", c)
	}
	inspect, err := agent.DockerClient.ContainerInspect(context.Background(), c.ID)
	if err != nil {
		return nil, fmt.Errorf("Cannot inspect container %v", err)
	}
	inspectConfig, err := json.Marshal(inspect)
	if err != nil {
		return nil, fmt.Errorf("Cannot decode %v", inspect)
	}

	networks, err := agent.readContainerNetworkInfo(containerID)
	if err != nil {
		return nil, err
	}
	ac := &AgentContainer{
		ID:        c.ID,
		Container: string(data),
		Config:    string(inspectConfig),
		Networks:  networks,
	}
	lock.Lock()
	if trackedContainers[containerID] == nil {
		ac.AgentContainerStartRetry = &AgentContainerStartRetry{LastError: ""}
	} else {
		ac.AgentContainerStartRetry = trackedContainers[containerID].AgentContainerStartRetry
	}
	lock.Unlock()
	return ac, nil
}

func (agent *Agent) initTrackedContainers() error {
	trackedContainers = make(map[string]*AgentContainer)
	containers, err := agent.managedContainers()
	if err != nil {
		return nil
	}
	for _, container := range containers {
		trackedContainer, err := agent.trackedContainer(container.ID)
		if err != nil {
			logger.Fatal(err)
		}
		lock.Lock()
		trackedContainers[container.ID] = trackedContainer
		lock.Unlock()
	}
	return nil
}

func (step *TaskStep) dockerPush(opts *DockerPushOpts) error {
	step.stream([]byte(fmt.Sprintf("\033[33m[PUSH]\033[0m Pushing docker image %s\n", opts.Image)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pushOpts := docker_types.ImagePushOptions{}
	if opts.InternalRegistryAuth {
		pushOpts.RegistryAuth = agent.internalRegistryAuthString()
	} else {
		if opts.ExternalAuth != nil {
			pushOpts.RegistryAuth = opts.ExternalAuth.registryAuthString(agent.ID)
		}
	}
	out, err := agent.DockerClient.ImagePush(ctx, opts.Image, pushOpts)
	if err != nil {
		return err
	}
	defer out.Close()
	scanner := bufio.NewScanner(out)
	wasInProgress := false
	for scanner.Scan() {
		j := jsonmessage.JSONMessage{}
		err := json.Unmarshal(scanner.Bytes(), &j)
		if err != nil {
			continue
		}
		if j.Error != nil {
			return fmt.Errorf("docker push failed: %s", j.Error.Message)
		}
		if j.Stream != "" {
			step.stream([]byte(j.Stream))
		}
		if j.Status != "" {
			if j.Progress != nil {
				wasInProgress = true
				step.stream([]byte(fmt.Sprintf("%s\r", j.Status)))
			} else {
				if wasInProgress {
					step.stream([]byte("\n"))
					wasInProgress = false
				}
				step.stream([]byte(fmt.Sprintf("%s\r\n", j.Status)))
			}
		}
	}
	logger.Infof("Image %s pushed to registry", opts.Image)
	return nil
}

func (agent *Agent) dockerPull(opts *DockerPullOpts) error {
	ctx := context.Background()
	pullOpts := docker_types.ImagePullOptions{}
	if opts.InternalRegistryAuth {
		pullOpts.RegistryAuth = agent.internalRegistryAuthString()
	} else {
		if opts.ExternalAuth != nil {
			pullOpts.RegistryAuth = opts.ExternalAuth.registryAuthString(agent.ID)
		}
	}
	out, err := agent.DockerClient.ImagePull(ctx, opts.Image, pullOpts)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err = io.Copy(ioutil.Discard, out); err != nil {
		return err
	}
	logger.Infof("New image pulled %s", opts.Image)
	return nil
}

func (agent *Agent) dockerCreate(opts *DockerCreateOpts) (*docker_types_container.ContainerCreateCreatedBody, error) {
	ctx := context.Background()
	config := &docker_types_container.Config{
		Image:  opts.PullOpts.Image,
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
		config.Entrypoint = docker_types_strslice.StrSlice{opts.Entrypoint}
	}
	if opts.Command != "" {
		s, err := shlex.Split(opts.Command)
		if err != nil {
			return nil, err
		}
		config.Cmd = s
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
	// must come from the supernetwork
	if opts.Networks != nil {
		dns, err := agent.weave.ContainerDNS()
		if err != nil {
			return nil, fmt.Errorf("Unable to retrieve DNS inventory IP: %s", err)
		}
		hostConfig.DNS = []string{dns}
		hostConfig.DNSSearch = []string{agent.superNetwork.config.DNSDomain}

	}
	networkingConfig := &docker_types_network.NetworkingConfig{}
	container, err := agent.DockerClient.ContainerCreate(ctx, config, hostConfig, networkingConfig, opts.Name)
	if err != nil {
		return nil, err
	}
	logger.Infof("New container created %s", container.ID)
	return &container, nil
}

func (agent *Agent) dockerStart(containerID string, waitForRunning int64, networks []*subNetwork, dnsName string) error {
	ctx := context.Background()
	startOpts := docker_types.ContainerStartOptions{}
	if err := agent.DockerClient.ContainerStart(ctx, containerID, startOpts); err != nil {
		if trackedContainers[containerID] != nil {
			trackedContainers[containerID].AgentContainerStartRetry.LastErrorTimestamp = time.Now().UnixNano()
			trackedContainers[containerID].AgentContainerStartRetry.LastError = err.Error()
			trackedContainers[containerID].AgentContainerStartRetry.FailureCount++
			agent.chSyncContainer <- containerID
		}
		return err
	}
	time.Sleep(100 * time.Millisecond)
	i, err := agent.DockerClient.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return fmt.Errorf("Error inspect newly create container %s: %+v", containerID, err)
	}
	if !i.State.Running {
		return fmt.Errorf("Container %s crashed just after start", containerID)
	}
	_, errAttach := agent.attachNetworks(containerID, networks, dnsName)
	if errAttach != nil {
		return errAttach
	}
	if waitForRunning > 0 {
		logger.Debugf("Wait %d seconds for container %s to start", waitForRunning, containerID)
		time.Sleep(time.Duration(waitForRunning) * time.Second)
		i, err := agent.DockerClient.ContainerInspect(context.Background(), containerID)
		if err != nil {
			return fmt.Errorf("Error inspect newly create container %s: %+v", containerID, err)
		}
		if !i.State.Running {
			if trackedContainers[containerID] != nil {
				trackedContainers[containerID].AgentContainerStartRetry.LastErrorTimestamp = time.Now().UnixNano()
				trackedContainers[containerID].AgentContainerStartRetry.LastError = fmt.Sprintf("Container %s exited with code %d", containerID, i.State.ExitCode)
				trackedContainers[containerID].AgentContainerStartRetry.FailureCount++
				agent.chSyncContainer <- containerID
			}
			return fmt.Errorf("Cannot start container %s, check this container logs", containerID)
		}
		if trackedContainers[containerID] != nil {
			trackedContainers[containerID].AgentContainerStartRetry.LastErrorTimestamp = 0.0
			trackedContainers[containerID].AgentContainerStartRetry.LastError = ""
			trackedContainers[containerID].AgentContainerStartRetry.FailureCount = 0
			agent.chSyncContainer <- containerID
		}

	}
	return nil
}

func (agent *Agent) dockerUnpause(containerID string) error {
	ctx := context.Background()
	if err := agent.DockerClient.ContainerUnpause(ctx, containerID); err != nil {
		return err
	}
	logger.Infof("Container %s unpaused", containerID)
	return nil
}

func (agent *Agent) dockerPause(containerID string) error {
	ctx := context.Background()
	if err := agent.DockerClient.ContainerPause(ctx, containerID); err != nil {
		return err
	}
	logger.Infof("Container %s paused", containerID)
	return nil
}

func (agent *Agent) dockerStop(containerID string, timeout time.Duration) error {
	ctx := context.Background()
	if err := agent.detachNetworks(containerID, nil); err != nil {
		return err
	}

	if err := agent.DockerClient.ContainerStop(ctx, containerID, &timeout); err != nil {
		return err
	}
	logger.Infof("Container %s stopped", containerID)
	return nil
}

func (agent *Agent) dockerRestart(containerID string, timeout time.Duration) error {
	ctx := context.Background()
	if err := agent.DockerClient.ContainerRestart(ctx, containerID, &timeout); err != nil {
		return err
	}
	logger.Infof("Container %s restarted", containerID)
	return nil
}

func (agent *Agent) dockerRemove(containerID string, opts *DockerRemoveOpts) error {
	// first unpause container as we cannot remove a paused container
	i, err := agent.DockerClient.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return fmt.Errorf("Error inspect container %s: %+v", containerID, err)
	}
	if i.State.Paused {
		err := agent.DockerClient.ContainerUnpause(context.Background(), containerID)
		if err != nil {
			return fmt.Errorf("Unable to unpause container %s before removal: %+v", containerID, err)
		}
	}
	removeOpts := docker_types.ContainerRemoveOptions{
		Force:         opts.Force,
		RemoveVolumes: opts.RemoveVolumes,
		RemoveLinks:   opts.RemoveLinks,
	}
	ctx := context.Background()
	if err := agent.DockerClient.ContainerRemove(ctx, containerID, removeOpts); err != nil {
		return err
	}
	deletetrackedContainer(containerID)
	logger.Infof("Container %s remove", containerID)
	return nil
}

// ReadDockerignore reads the .dockerignore file in the context directory and
// returns the list of paths to exclude
func ReadDockerignore(contextDir string) ([]string, error) {
	var excludes []string

	f, err := os.Open(filepath.Join(contextDir, ".dockerignore"))
	switch {
	case os.IsNotExist(err):
		return excludes, nil
	case err != nil:
		return nil, err
	}
	defer f.Close()

	return dockerignore.ReadAll(f)
}

// TrimBuildFilesFromExcludes removes the named Dockerfile and .dockerignore from
// the list of excluded files. The daemon will remove them from the final context
// but they must be in available in the context when passed to the API.
func TrimBuildFilesFromExcludes(excludes []string, dockerfile string, dockerfileFromStdin bool) []string {
	if keep, _ := fileutils.Matches(".dockerignore", excludes); keep {
		excludes = append(excludes, "!.dockerignore")
	}
	if keep, _ := fileutils.Matches(dockerfile, excludes); keep && !dockerfileFromStdin {
		excludes = append(excludes, "!"+dockerfile)
	}
	return excludes
}

func (step *TaskStep) dockerBuild(opts *DockerBuildOpts) error {
	defer func() {
		if opts.ClearBuildDir {
			os.RemoveAll(opts.Path)
		}
	}()
	logger.Infof("Building image %s", opts.Tag)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	contextDir, relDockerfile, err := builder.GetContextFromLocalDir(opts.Path, fmt.Sprintf("%s/%s", opts.Path, "Dockerfile"))
	if err != nil {
		return fmt.Errorf("unable to prepare context: %s", err)
	}
	relDockerfile, err = archive.CanonicalTarNameForPath(relDockerfile)
	if err != nil {
		return fmt.Errorf("cannot canonicalize dockerfile path %s: %v", relDockerfile, err)
	}
	excludes, _ := ReadDockerignore(opts.Path)
	excludes = TrimBuildFilesFromExcludes(excludes, relDockerfile, false)
	tarOptions := &archive.TarOptions{
		ExcludePatterns: excludes,
		ChownOpts:       &archive.TarChownOptions{UID: 0, GID: 0},
	}
	buildCtx, err := archive.TarWithOptions(contextDir, tarOptions)
	if err != nil {
		return err
	}
	defer buildCtx.Close()
	var body io.Reader
	progressOutput := streamformatter.NewStreamFormatter().NewProgressOutput(step.Task.streamer.ioWriter, true)
	body = progress.NewProgressReader(buildCtx, progressOutput, 0, "", "Sending build context to Docker daemon")
	buildOpts := docker_types.ImageBuildOptions{
		NoCache:     true,
		Dockerfile:  relDockerfile,
		Tags:        []string{opts.Tag},
		Remove:      opts.RemoveIntermediateContainers,
		ForceRemove: opts.ForceRemoveIntermediateContainers,
		PullParent:  true,
	}
	step.stream([]byte(fmt.Sprintf("\033[33m[BUILD]\033[0m Building docker image %s\n", opts.Tag)))
	buildResponse, err := agent.DockerClient.ImageBuild(ctx, body, buildOpts)
	if err != nil {
		return err
	}
	defer buildResponse.Body.Close()
	buf := make([]byte, 1024)
	for {
		select {
		case <-step.Task.cancelCh:
			return fmt.Errorf("Docker build cancelled")
		default:
		}
		read, err := buildResponse.Body.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if read > 0 {
			j := jsonmessage.JSONMessage{}
			err := json.Unmarshal([]byte(buf[:read]), &j)
			if err != nil {
				continue
			}
			step.stream([]byte(j.Stream))
		}
	}
	logger.Infof("Image %s built", opts.Tag)
	return nil
}

func (agent *Agent) dockerTerminal(opts *DockerTerminalOpts) error {
	// fetch terminal info from pikacloud API
	terminal, err := pikacloudClient.Terminal(agent.ID, opts.Tid)
	if err != nil {
		return err
	}

	// connect to websocket
	quit := false
	wsURLParams := strings.Split(wsURL, "://")
	scheme := wsURLParams[0]
	addr := wsURLParams[1]
	path := fmt.Sprintf("/_ws/hub/agent/%s:%s:%s/", agent.ID, opts.Tid, terminal.Token)
	u := url.URL{Scheme: scheme, Host: addr, Path: path}
	logger.Infof("connecting to %s", u.String())

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 3 * time.Second
	c, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("Error dialing %s%s: %s", wsURL, path, err)
	}
	ctx := context.Background()
	configExec := docker_types.ExecConfig{
		Tty:          true,
		AttachStdin:  true,
		AttachStderr: true,
		AttachStdout: true,
		Detach:       false,
		Env:          []string{"TERM=xterm"},
		Cmd:          []string{"bash"},
	}
	execCreateResponse, err := agent.DockerClient.ContainerExecCreate(ctx, opts.Cid, configExec)
	if err != nil {
		return fmt.Errorf("Cannot create container %s exec %+v: %s", opts.Cid, configExec, err)
	}

	execAttachResponse, errAttach := agent.DockerClient.ContainerExecAttach(ctx, execCreateResponse.ID, configExec)
	if errAttach != nil {
		return errAttach
	}
	runningTerminalsList[opts] = true
	defer func() {
		logger.Info("Defer fx terminal")
		errDeleteTerminal := terminal.Delete(pikacloudClient)
		if errDeleteTerminal != nil {
			logger.Info("Cannot delete terminal in API: %+v", errDeleteTerminal)
		}
		data, errInspect := agent.DockerClient.ContainerExecInspect(ctx, execCreateResponse.ID)
		if errInspect == nil {
			if data.Running {
				if strings.HasSuffix(runtime.GOOS, "darwin") || isXhyve {
					// https://www.reddit.com/r/docker/comments/6d8yt3/killing_a_process_from_docker_exec_on_os_x/
					// https://gist.github.com/bschwind/7ef38e2918c43bd7ee23d86dad86db7d
					logger.Info("Killing leaked process (darwin)")
					command := fmt.Sprintf("echo kill -9 %d > %s", data.Pid, xhyveTTY)
					cmd := exec.Command("/bin/sh", "-c", command)
					errCommand := cmd.Run()
					if errCommand != nil {
						logger.Infof("Failed to kill PID %d in the xhyve VM: %v", data.Pid, errCommand)
					} else {
						logger.Infof("Killed leaked PID %d in the xhyve VM", data.Pid)
					}
				} else {
					logger.Info("Killing leaked process (unix)")
					command := fmt.Sprintf("kill -9 %d", data.Pid)
					cmd := exec.Command("/bin/sh", "-c", command)
					errCommand := cmd.Run()
					if errCommand != nil {
						logger.Infof("Failed to kill PID %d locally: %v", data.Pid, errCommand)
					} else {
						logger.Infof("Leaked PID %d killed locally", data.Pid)
					}
				}
			}
		}
		execAttachResponse.Close()
		logger.Infof("Exec session %s closed", execCreateResponse.ID)
		c.Close()
		logger.Infof("Websocket connection closed %s", opts.Tid)
		delete(runningTerminalsList, opts)
		return

	}()
	go func() {
		defer func() {
			quit = true
		}()
		for {
			if quit {
				return
			}
			messageType, reader, errReader := c.NextReader()
			if errReader != nil {
				logger.Info("Unable to grab next reader")
				quit = true
				return
			}
			if messageType == websocket.TextMessage {
				logger.Info("Unexpected text message")
				c.WriteMessage(websocket.TextMessage, []byte("Unexpected text message"))
				quit = true
				continue
			}
			dataTypeBuf := make([]byte, 1)
			read, errR := reader.Read(dataTypeBuf)
			if errR != nil {
				logger.Infof("Unable to read message type from reader %+v", errR)
				c.WriteMessage(websocket.TextMessage, []byte("Unable to read message type from reader"))
				quit = true
				return
			}

			if read != 1 {
				logger.Infof("Unexpected number of bytes read %+v", read)
				quit = true
				return
			}

			switch dataTypeBuf[0] {
			case 0:
				copied, errCopy := io.Copy(execAttachResponse.Conn, reader)
				if errCopy != nil {
					logger.Infof("Error after copying %d bytes %+v", copied, errCopy)
				}
			case 1:
				decoder := json.NewDecoder(reader)
				resizeMessage := windowSize{}
				errResizeMessage := decoder.Decode(&resizeMessage)
				if errResizeMessage != nil {
					c.WriteMessage(websocket.TextMessage, []byte("Error decoding resize message: "+errResizeMessage.Error()))
					continue
				}
				logger.Infof("Resizing terminal %+v", resizeMessage)
				errResize := agent.ContainerExecResize(execCreateResponse.ID, resizeMessage.Rows, resizeMessage.Cols)
				if err != nil {
					logger.Infof("Unable to resize terminal %+v", errResize)
				}
			default:
				logger.Infof("Unknown data type %+v", dataTypeBuf)
			}
		}
	}()
	go func() {
		for {
			if quit {
				return
			}
			buf := make([]byte, 1024)
			read, err := execAttachResponse.Conn.Read(buf)
			if err != nil {
				c.WriteMessage(websocket.TextMessage, []byte(err.Error()))
				fmt.Printf("Unable to read from pty/cmd")
				quit = true
				return
			}
			c.WriteMessage(websocket.BinaryMessage, []byte(buf[:read]))
		}
	}()
	for {
		if quit {
			return nil
		}
		data, errD := agent.DockerClient.ContainerExecInspect(ctx, execCreateResponse.ID)
		if errD != nil {
			quit = true
			return errD
		}
		if !data.Running {
			quit = true
			return fmt.Errorf("bad exit code(%d)", data.ExitCode)
		}
		time.Sleep(1 * time.Second)
	}
}

func (agent *Agent) trackedDockerContainersSyncer() {
	logger.Debug("Starting docker containers syncer")

	defer func() {
		logger.Debug("Docker containers syncer exited")
	}()

	for {
		select {
		case containerID := <-agent.chRegisterContainer:
			trackedContainer, err := agent.trackedContainer(containerID)
			if err != nil {
				// logger.Debugf("Cannot generate trackedContainer for register: %+v", err)
				continue
			}
			isManaged, err := agent.isManagedContainer(containerID)
			if err != nil {
				continue
			}
			if isManaged {
				lock.Lock()
				trackedContainers[containerID] = trackedContainer
				lock.Unlock()
				errSync := agent.syncDockerContainers([]*AgentContainer{trackedContainer})
				if errSync != nil {
					logger.Errorf("Cannot sync container %s: %+v", containerID, errSync)
					agent.forceSyncTrackedDockerContainers()
					continue
				}
				logger.WithFields(logrus.Fields{"cid": containerID}).Debug("Container synced")
			}
		case containerID := <-agent.chDeregisterContainer:
			deletetrackedContainer(containerID)
		case containerID := <-agent.chSyncContainer:
			agent.syncDockerContainer(containerID)
		}
	}
}

func (agent *Agent) syncDockerContainer(containerID string) error {
	trackedContainer, err := agent.trackedContainer(containerID)
	if err != nil {
		return err
	}
	isManaged, err := agent.isManagedContainer(containerID)
	if err != nil {
		return err
	}
	if isManaged {
		trackedContainers[containerID] = trackedContainer
		errSync := agent.syncDockerContainers([]*AgentContainer{trackedContainer})
		if errSync != nil {
			logger.Errorf("Cannot sync container %s: %+v", containerID, errSync)
			agent.forceSyncTrackedDockerContainers()
			return nil
		}
		logger.WithFields(logrus.Fields{"cid": containerID}).Debug("Container synced (update)")
	}
	return nil
}

func deletetrackedContainer(containerID string) error {
	lock.Lock()
	if trackedContainers[containerID] != nil {
		delete(trackedContainers, containerID)
		err := agent.unsyncDockerContainer(containerID)
		if err != nil {
			logger.Debugf("Cannot unsync container %s: %+v", containerID, err)
			agent.forceSyncTrackedDockerContainers()
			return nil
		}
		logger.WithFields(logrus.Fields{"cid": containerID}).Debug("Container unsynced")
	}
	lock.Unlock()
	return nil
}

func (agent *Agent) forceSyncTrackedDockerContainers() error {
	var containers []*AgentContainer
	for _, container := range trackedContainers {
		containers = append(containers, container)
	}
	err := agent.syncDockerContainers(containers)
	if err != nil {
		return err
	}
	return nil
}

func (agent *Agent) syncDockerContainers(containers []*AgentContainer) error {
	uri := fmt.Sprintf("run/agents/%s/docker/containers/", agent.ID)
	status, err := pikacloudClient.Post(uri, containers, nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("Failed to sync docker containers: %d", status)
	}
	return nil
}

func (agent *Agent) unsyncDockerContainer(containerID string) error {
	deleteContainerURI := fmt.Sprintf("run/agents/%s/docker/containers/%s/", agent.ID, containerID)
	_, err := pikacloudClient.Delete(deleteContainerURI, nil, nil)
	if err != nil {
		return fmt.Errorf("Cannot delete container %s in API: %+v", containerID, err)
	}
	return nil
}

func (agent *Agent) isManagedContainer(containerID string) (bool, error) {
	container, err := agent.DockerClient.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return false, err
	}
	expectedLabel := "pikacloud.container.id"
	containerConfigID := container.Config.Labels[expectedLabel]
	if containerConfigID == "" {
		return false, nil
	}
	return true, nil
}

func (agent *Agent) managedContainers() ([]docker_types.Container, error) {
	containers, err := agent.DockerClient.ContainerList(context.Background(), docker_types.ContainerListOptions{})
	if err != nil {
		return nil, err
	}
	managedContainersList := []docker_types.Container{}
	for _, container := range containers {
		ok, err := agent.isManagedContainer(container.ID)
		if err != nil {
			logger.Debugf("Unable to check if container %s is a managedContainer: %+v", container.ID, err)
			continue
		}
		if !ok {
			continue
		}
		managedContainersList = append(managedContainersList, container)
	}
	return managedContainersList, nil
}

func (agent *Agent) containerLogger(containerID string, streamFromNow bool) error {
	container, err := agent.DockerClient.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return err
	}
	containerConfigID := container.Config.Labels["pikacloud.container.id"]
	// Stream logs only for pikacloud containers.
	if containerConfigID == "" {
		return nil
	}
	// Stream logs only if stream logging is enabled in container config
	if container.Config.Labels["pikacloud.container.stream_logs"] != "true" {
		return nil
	}
	containerLogsOpts := docker_types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	}
	layout := time.RFC3339Nano
	t, err := time.Parse(layout, container.State.FinishedAt)
	if err != nil {
		return err
	}
	if !t.IsZero() && !streamFromNow {
		containerLogsOpts.Since = container.State.FinishedAt
	}
	if streamFromNow {
		containerLogsOpts.Since = time.Now().Format(layout)
	}
	logger.Debugf("Starting log streamer for %s", container.ID)
	ctx := context.Background()
	logReader, err := agent.DockerClient.ContainerLogs(ctx, container.ID, containerLogsOpts)
	if err != nil {
		return err
	}

	go func() {
		defer func() {
			logReader.Close()
			logger.Debugf("Log streamer closed for %s", container.ID)
		}()
		scanner := bufio.NewScanner(logReader)
		for scanner.Scan() {
			entry := &StreamEntry{
				Cid:               stringid.TruncateID(container.ID),
				ContainerConfigID: containerConfigID,
				Msg:               []byte(scanner.Text()),
				Aid:               agent.ID,
			}
			streamer.writeEntry(entry)
		}
	}()
	return nil
}

func (agent *Agent) parseContainerEvent(msg docker_types_event.Message) error {
	containerID := msg.ID
	switch msg.Action {
	case "create":
		agent.chRegisterContainer <- containerID
	case "start":
		agent.chSyncContainer <- containerID
		agent.containerLogger(containerID, false)
	case "destroy":
		agent.chDeregisterContainer <- containerID
	default:
		agent.chSyncContainer <- containerID
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

func waitDockerEvents(events <-chan docker_types_event.Message, errs <-chan error) error {
	for {
		select {
		case dMsg := <-events:
			err := agent.parseDockerEvent(dMsg)
			if err != nil {
				return err
			}
		case dErr := <-errs:
			if dErr != nil {
				return dErr
			}
		}
	}
}

func (agent *Agent) listenDockerEvents() error {
	ctx := context.Background()
	//messages := make(chan events.Message)
	//errs := make(chan error, 1)
	for {
		events, errs := agent.DockerClient.Events(ctx, docker_types.EventsOptions{})
		err := waitDockerEvents(events, errs)
		if err != nil {
			logger.Infof("Error reading docker events: %s", err)
			time.Sleep(3 * time.Second)
		}
	}
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
		_, _, errImage := agent.DockerClient.ImageInspectWithRaw(context.Background(), createOpts.PullOpts.Image)
		if errImage != nil {
			// no local image, try pulling it
			err = agent.dockerPull(createOpts.PullOpts)
			if err != nil {
				return err
			}
		} else {
			if createOpts.PullOpts.AlwaysPull {
				err = agent.dockerPull(createOpts.PullOpts)
				if err != nil {
					return err
				}
			}
		}
		containerCreated, err := agent.dockerCreate(&createOpts)
		if err != nil {
			return err
		}
		if err := agent.dockerStart(containerCreated.ID, createOpts.WaitForRunning, createOpts.Networks, createOpts.DNSName); err != nil {
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
	case "tag":
		var tagOpts = DockerImageTagOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &tagOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker tag: %s (%v)", err, step.PluginConfig)
		}
		if err := agent.DockerClient.ImageTag(context.Background(), tagOpts.Source, tagOpts.Target); err != nil {
			return fmt.Errorf("Unable to tag image: %+v", err)
		}
		return nil
	case "build":
		var buildOpts = DockerBuildOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &buildOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker build: %s (%v)", err, step.PluginConfig)
		}
		err = step.dockerBuild(&buildOpts)
		if err != nil {
			return err
		}
		return nil
	case "push":
		var pushOpts = DockerPushOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &pushOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker push: %s (%v)", err, step.PluginConfig)
		}
		err = step.dockerPush(&pushOpts)
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
			return fmt.Errorf("Bad config for docker start: %s (%v)", err, step.PluginConfig)
		}
		err = agent.dockerStart(startOpts.ID, startOpts.WaitForRunning, startOpts.Networks, startOpts.DNSName)
		if err != nil {
			return err
		}
		return nil
	case "stop":
		var stopOpts = DockerStopOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &stopOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker stop: %s (%v)", err, step.PluginConfig)
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
	case "terminal":
		var terminalOpts = DockerTerminalOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &terminalOpts)
		if err != nil {
			return fmt.Errorf("Bad config for docker terminal: %s (%v)", err, step.PluginConfig)
		}
		terminalOpts.Tid = step.Task.ID
		err = agent.dockerTerminal(&terminalOpts)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("Unknown step method %s", step.Method)
	}
}
