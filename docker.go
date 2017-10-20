package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fernet/fernet-go"

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

// DockerPullOpts describes docker pull options
type DockerPullOpts struct {
	Image        string                `json:"image"`
	AlwaysPull   bool                  `json:"always_pull"`
	ExternalAuth *ExternalAuthPullOpts `json:"external_registry_auth"`
}

func (e *ExternalAuthPullOpts) registryAuthString(aid string) string {
	key := base64.StdEncoding.EncodeToString([]byte(aid))
	k := fernet.MustDecodeKeys(key)
	password := fernet.VerifyAndDecrypt([]byte(e.Password), 60*time.Second, k)
	data := []byte(fmt.Sprintf("{\"username\": \"%s\", \"password\": \"%s\"}", e.Login, string(password)))
	str := base64.StdEncoding.EncodeToString(data)
	return str
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
	ID             string `json:"id"`
	WaitForRunning int64  `json:"wait_for_running"`
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
	Tag           string `json:"tag"`
	Path          string `json:"path"`
	ClearBuildDir bool   `json:"clear_build_dir"`
	BuildID       string `json:"build_id"`
	RemoveImage   bool   `json:"remove_image_after_build"`
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
}

type syncDockerContainersOptions struct {
	ContainersID []string
}

type containerLog struct {
	containerID string
	msg         []byte
}

// NewDockerClient creates a new docker client
func NewDockerClient() *docker_client.Client {

	dockerClient, err := docker_client.NewEnvClient()
	if err != nil {
		logger.Fatal(err)
	}
	_, err = dockerClient.ServerVersion(context.Background())
	if err != nil {
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

	ac := &AgentContainer{
		ID:        c.ID,
		Container: string(data),
		Config:    string(inspectConfig),
	}
	lock.Lock()
	if trackedContainers[containerID] == nil {
		ac.AgentContainerStartRetry = &AgentContainerStartRetry{LastError: "caca"}
	} else {
		ac.AgentContainerStartRetry = trackedContainers[containerID].AgentContainerStartRetry
	}
	lock.Unlock()
	return ac, nil
}

func (agent *Agent) initTrackedContainers() error {
	trackedContainers = make(map[string]*AgentContainer)
	containers, err := agent.DockerClient.ContainerList(
		context.Background(),
		docker_types.ContainerListOptions{
			All: true,
		})
	if err != nil {
		return err
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
func (agent *Agent) dockerPull(opts *DockerPullOpts) error {
	ctx := context.Background()
	pullOpts := docker_types.ImagePullOptions{}
	if opts.ExternalAuth != nil {
		pullOpts.RegistryAuth = opts.ExternalAuth.registryAuthString(agent.ID)
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
	networkingConfig := &docker_types_network.NetworkingConfig{}
	container, err := agent.DockerClient.ContainerCreate(ctx, config, hostConfig, networkingConfig, opts.Name)
	if err != nil {
		return nil, err
	}
	logger.Infof("New container created %s", container.ID)
	return &container, nil
}

func (agent *Agent) dockerStart(containerID string, waitForRunning int64) error {
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
	logger.Infof("New container started %s", containerID)
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

// IDPair is a UID and GID pair
type IDPair struct {
	UID int
	GID int
}

func (step *TaskStep) dockerBuild(opts *DockerBuildOpts) error {
	defer os.RemoveAll(opts.Path)
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
	excludes, err := ReadDockerignore(opts.Path)
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
		NoCache:    true,
		Dockerfile: relDockerfile,
		Tags:       []string{opts.Tag},
		Remove:     opts.RemoveImage,
		PullParent: true,
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
	// ctxWithTimeout, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	// defer ctxCancel()
	execCreateResponse, err := agent.DockerClient.ContainerExecCreate(ctx, opts.Cid, configExec)
	if err != nil {
		return fmt.Errorf("Cannot create container %s exec %+v: %s", opts.Cid, configExec, err)
	}
	// configStart := docker_types.ExecStartCheck{
	// 	Detach: configExec.Detach,
	// 	Tty:    configExec.Tty,
	// }

	// go func() error {
	// 	errStart := agent.DockerClient.ContainerExecStart(ctxWithTimeout, execCreateResponse.ID, configStart)
	// 	if errStart != nil {
	// 		return errStart
	// 	}
	// 	return nil
	// }()

	execAttachResponse, errAttach := agent.DockerClient.ContainerExecAttach(ctx, execCreateResponse.ID, configExec)
	if errAttach != nil {
		return errAttach
	}

	// stdIn, stdOut, _ := term.StdStreams()
	// stdInFD, _ := term.GetFdInfo(stdIn)
	// stdOutFD, _ := term.GetFdInfo(stdOut)
	//
	// oldInState, _ := term.SetRawTerminal(stdInFD)
	// oldOutState, _ := term.SetRawTerminalOutput(stdOutFD)
	//
	// defer term.RestoreTerminal(stdInFD, oldInState)
	// defer term.RestoreTerminal(stdOutFD, oldOutState)

	// var stdin io.Writer
	// r, w := io.Pipe()
	// defer w.Close()
	// var stdout io.Writer
	// var stderr io.Writer

	// var buff bytes.Buffer
	// // if stderr == nil {
	// // 	stderr = &buff
	// // }
	// go func() error {
	// 	if err := execPipe(execAttachResponse, nil, w, w); err != nil {
	// 		return err
	// 	}
	// go func() error {
	// 	for {
	//
	// 		fmt.Println(data)
	// 	}
	// }()
	// 	data, err := agent.DockerClient.ContainerExecInspect(ctx, execCreateResponse.ID)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	if data.ExitCode != 0 {
	// 		// if so use the buffer that may have been assigned to the
	// 		// streams to give message better error handling
	// 		return fmt.Errorf("bad exit code(%d): %s", data.ExitCode, buff.String())
	// 	}
	// 	return nil
	// }()
	//
	// fmt.Println("no wait")
	// // cmd := exec.Command("docker", "exec", "-it", opts.ID, "bash")
	// // // cmd := exec.Command("htop")
	// // cmd.Env = append(os.Environ(), "TERM=xterm")
	// // tty, err := pty.Start(cmd)
	// // if err != nil {
	// // 	c.WriteMessage(websocket.TextMessage, []byte(err.Error()))
	// // 	return fmt.Errorf("Unable to start pty/cmd %+v", err)
	// // }
	runningTerminalsList[opts] = true
	defer func() {
		logger.Info("Defer fx terminal")
		data, errInspect := agent.DockerClient.ContainerExecInspect(ctx, execCreateResponse.ID)
		if errInspect == nil {
			if data.Running {
				// configKillExec := docker_types.ExecConfig{
				// 	Tty:          false,
				// 	AttachStdin:  false,
				// 	AttachStderr: false,
				// 	AttachStdout: false,
				// 	Detach:       false,
				// 	Env:          []string{"TERM=xterm"},
				// }
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
					// configKillExec.Cmd = []string{"kill", "-9", fmt.Sprintf("%d", data.Pid)}
					// killCreateResponse, errKillCreate := agent.DockerClient.ContainerExecCreate(ctx, opts.ID, configKillExec)
					// if err == errKillCreate {
					// 	killStartResponse, errKillStart := agent.DockerClient.ContainerExecAttach(ctx, killCreateResponse.ID, configKillExec)
					// 	defer killStartResponse.Close()
					// 	if errKillStart == nil {
					// 		dataKill, errKillInspect := agent.DockerClient.ContainerExecInspect(ctx, execCreateResponse.ID)
					// 		if errKillInspect != nil {
					// 			logger.Infof("Cannot kill leaked process %d", data.Pid)
					// 		} else {
					// 			if dataKill.ExitCode == 0 {
					// 				logger.Infof("Successfully killed leaked process %d", data.Pid)
					// 			} else {
					// 				logger.Infof("Cannot kill leaked process: 'kill -9 %d' exit code is %d'", data.Pid, dataKill.ExitCode)
					// 			}
					// 		}
					// 	}
					//}
				}
			}
		}

		// logger.Infof("Garbage collecting Terminal %s", opts.Task.ID)
		// cmd.Process.Kill()
		// cmd.Process.Wait()
		// logger.Infof("Terminal command killed %s", opts.Task.ID)
		// tty.Close()
		// logger.Infof("TTY closed %s", opts.Task.ID)
		execAttachResponse.Close()
		logger.Infof("Exec session %s closed", execCreateResponse.ID)
		c.Close()
		logger.Infof("Websocket connection closed %s", opts.Tid)
		delete(runningTerminalsList, opts)
		err := terminal.Delete(pikacloudClient)
		if err != nil {
			logger.Info("Cannot delete terminal in API: %+v", err)
		}

		return

	}()
	// quit := false
	//go io.Copy(execAttachResponse.Conn, execAttachResponse.Conn.)
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
				// _, _, errno := syscall.Syscall(
				// 	syscall.SYS_IOCTL,
				// 	// tty.Fd(),
				// 	syscall.TIOCSWINSZ,
				// 	uintptr(unsafe.Pointer(&resizeMessage)),
				// )
				// if errno != 0 {
				// 	logger.Infof("Unable to resize terminal %+v", errno)
				// }
			default:
				logger.Infof("Unknown data type %+v", dataTypeBuf)
			}
			// if err != nil {
			// 	logger.Info("Error reading message from WS", err)
			// 	return
			// }
			//
			// _, err = execAttachResponse.Conn.Write(message)
			// if err != nil {
			// 	logger.Info("Error writing message to container", err)
			// 	return
			// }

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
	// wr := WSReaderWriter{c}
	// io.Copy(wr)
	// // go func() {
	// for {
	// 	if quit {
	// 		return nil
	// 	}
	//
	// 	// fmt.Println(stdout)
	// 	buf := new(bytes.Buffer)
	// 	buf.ReadFrom(r)
	// 	s := buf.String()
	// 	if s != "" {
	// 		fmt.Println("----", s)
	// 	}
	// 	// buf := make([]byte, 1024)
	// 	// // io.Copy(os.Stdout, r)
	// 	// read, _ := r.Read(buf)
	// 	// fmt.Println(buf, read)
	// 	// if err != nil {
	// 	// 	c.WriteMessage(websocket.TextMessage, []byte(err.Error()))
	// 	// 	quit = true
	// 	// 	fmt.Printf("Unable to read from pty/cmd")
	// 	// 	return
	// 	// }
	// 	// c.WriteMessage(websocket.BinaryMessage, []byte(buf[:read]))
	// 	// c.WriteMessage(websocket.BinaryMessage, []byte(stdout))
	//
	// }
	// }()

	// for {
	// 	if quit {
	// 		return nil
	// 	}
	// 	// q := <-quit
	// 	// fmt.Println("pass")
	// 	// if q {
	// 	// 	fmt.Println("Got quit in channel from tty reader goroutine")
	// 	// 	return nil
	// 	// }
	// 	messageType, reader, err := c.NextReader()
	// 	if err != nil {
	// 		logger.Info("Unable to grab next reader")
	// 		quit = true
	// 		logger.Info("Sending true in quit")
	// 		return nil
	// 	}
	//
	// 	if messageType == websocket.TextMessage {
	// 		logger.Info("Unexpected text message")
	// 		c.WriteMessage(websocket.TextMessage, []byte("Unexpected text message"))
	// 		quit = true
	// 		continue
	// 	}
	// 	dataTypeBuf := make([]byte, 1)
	// 	read, err := reader.Read(dataTypeBuf)
	// 	if err != nil {
	// 		logger.Infof("Unable to read message type from reader %+v", err)
	// 		c.WriteMessage(websocket.TextMessage, []byte("Unable to read message type from reader"))
	// 		quit = true
	// 		return nil
	// 	}
	//
	// 	if read != 1 {
	// 		logger.Infof("Unexpected number of bytes read %+v", read)
	// 		quit = true
	// 		return nil
	// 	}
	//
	// 	switch dataTypeBuf[0] {
	// 	case 0:
	// 		copied, err := io.Copy(tty, reader)
	// 		if err != nil {
	// 			logger.Infof("Error after copying %d bytes %+v", copied, err)
	// 		}
	// 	case 1:
	// 		decoder := json.NewDecoder(reader)
	// 		resizeMessage := windowSize{}
	// 		err := decoder.Decode(&resizeMessage)
	// 		if err != nil {
	// 			c.WriteMessage(websocket.TextMessage, []byte("Error decoding resize message: "+err.Error()))
	// 			continue
	// 		}
	// 		logger.Infof("Resizing terminal %+v", resizeMessage)
	// 		_, _, errno := syscall.Syscall(
	// 			syscall.SYS_IOCTL,
	// 			tty.Fd(),
	// 			syscall.TIOCSWINSZ,
	// 			uintptr(unsafe.Pointer(&resizeMessage)),
	// 		)
	// 		if errno != 0 {
	// 			logger.Infof("Unable to resize terminal %+v", errno)
	// 		}
	// 	default:
	// 		logger.Infof("Unknown data type %+v", dataTypeBuf)
	// 	}
	// }
	// // for {
	// 	buf := make([]byte, 1024)
	// 	read, _ := tty.Read(buf)
	// 	if err != nil {
	// 		c.WriteMessage(websocket.TextMessage, []byte(err.Error()))
	// 		fmt.Printf("Unable to read from pty/cmd")
	// 		return nil
	// 	}
	// 	c.WriteMessage(websocket.BinaryMessage, []byte(buf[:read]))
	// }

	// }()
	// ping terminal endpoint
	//
	// for {
	//
	// 	time.Sleep(3 * time.Second)
	// }
	// ctx := context.Background()
	// if err := agent.DockerClient.ContainerRestart(ctx, containerID, &timeout); err != nil {
	// 	return err
	// }
	// logger.Infof("Container %s restarted", containerID)
	// return nil
	// return nil
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
				logger.Infof("Cannot sync docker info: %+v", err)
			} else {
				dockerInfoState = info
				logger.Debug("Sync docker info OK")
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
				logger.Errorf("Cannot generate trackerContainer: %+v", err)
				continue
			}
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
		case containerID := <-agent.chDeregisterContainer:
			lock.Lock()
			delete(trackedContainers, containerID)
			lock.Unlock()
			err := agent.unsyncDockerContainer(containerID)
			if err != nil {
				logger.Errorf("Cannot unsync container %s: %+v", containerID, err)
				agent.forceSyncTrackedDockerContainers()
				continue
			}
			logger.WithFields(logrus.Fields{"cid": containerID}).Debug("Container unsynced")
		case containerID := <-agent.chSyncContainer:
			trackedContainer, err := agent.trackedContainer(containerID)
			if err != nil {
				logger.Errorf("Cannot generate trackerContainer: %+v", err)
				continue
			}
			trackedContainers[containerID] = trackedContainer
			errSync := agent.syncDockerContainers([]*AgentContainer{trackedContainer})
			if errSync != nil {
				logger.Errorf("Cannot sync container %s: %+v", containerID, errSync)
				agent.forceSyncTrackedDockerContainers()
				continue
			}
			logger.WithFields(logrus.Fields{"cid": containerID}).Debug("Container synced (update)")
		}
	}
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
	status, err := agent.Client.Post(uri, containers, nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("Failed to sync docker containers: %d", status)
	}
	return nil
}

// containersSyncList := []*AgentContainer{}
// for _, container := range trackedContainers {
// 	if len(containersID) > 0 {
// 		if containersID[container.ID] {
// 			containersSyncList = append(containersSyncList, container)
// 		}
// 	} else {
// 		containersSyncList = append(containersSyncList, container)
// 	}
// }
// if len(containersSyncList) > 0 {
// 	uri := fmt.Sprintf("run/agents/%s/docker/containers/", agent.ID)
// 	status, err := agent.Client.Post(uri, containersSyncList, nil)
// 	if err != nil {
// 		return err
// 	}
// 	if status != 200 {
// 		return fmt.Errorf("Failed to sync docker containers: %d", status)
// 	}
// 	logger.Infof("Sync docker %d containers of %d OK", len(containersSyncList), len(trackedContainers))
// }
//
// return nil

// }

func (agent *Agent) unsyncDockerContainer(containerID string) error {
	deleteContainerURI := fmt.Sprintf("run/agents/%s/docker/containers/%s/", agent.ID, containerID)
	_, err := agent.Client.Delete(deleteContainerURI, nil, nil)
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
		return false, fmt.Errorf("No label pikacloud.container.id")
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
	return nil
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
		if err := agent.dockerStart(containerCreated.ID, createOpts.WaitForRunning); err != nil {

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
		err = agent.dockerStart(startOpts.ID, startOpts.WaitForRunning)
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
