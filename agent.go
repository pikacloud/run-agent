package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	update "github.com/inconshreveable/go-update"
	docker_client "github.com/moby/moby/client"
	"github.com/pikacloud/gopikacloud"
)

// CreateAgentOptions represents the agent Create() options
type CreateAgentOptions struct {
	Hostname   string   `json:"hostname"`
	Labels     []string `json:"labels,omitempty"`
	Localtime  int      `json:"localtime"`
	Version    string   `json:"version"`
	OS         string   `json:"os"`
	Arch       string   `json:"arch"`
	IP         string   `json:"ip"`
	Interfaces []string `json:"interfaces"`
}

// PingAgentOptions represents the agent Ping() options
type PingAgentOptions struct {
	Metrics          *Metrics `json:"metrics,omitempty"`
	RunningTasks     []string `json:"running_tasks,omitempty"`
	RunningTerminals []string `json:"running_terminals,omitempty"`
	Localtime        int      `json:"localtime"`
	NumGoroutines    int      `json:"num_goroutines"`
}

// Agent describes the agent
type Agent struct {
	Client                *Client
	DockerClient          *docker_client.Client
	ID                    string            `json:"aid"`
	PingURL               string            `json:"ping_url"`
	TTL                   int               `json:"ttl"`
	Hostname              string            `json:"hostname"`
	Labels                []string          `json:"labels"`
	Localtime             int               `json:"localtime"`
	User                  *gopikacloud.User `json:"user"`
	chRegisterContainer   chan string
	chDeregisterContainer chan string
	chSyncContainer       chan string
}

func localtime() int {
	return int(time.Now().Unix())
}

func makeLabels(labels string) []string {
	labelsList := strings.Split(labels, ",")
	ret := make([]string, len(labelsList))
	for idx, l := range labelsList {
		if l == "" {
			return nil
		}
		l = strings.TrimSpace(l)
		l = strings.Join(strings.Fields(l), "_")
		ret[idx] = l
	}
	return ret
}

// NewAgent create a new agent
func NewAgent(apiToken string, hostname string, labels []string) *Agent {
	return &Agent{
		Hostname:              hostname,
		Client:                NewClient(apiToken),
		DockerClient:          NewDockerClient(),
		Labels:                labels,
		chRegisterContainer:   make(chan string),
		chDeregisterContainer: make(chan string),
		chSyncContainer:       make(chan string),
	}
}

// Register an agent
func (agent *Agent) Register() error {
	req, err := http.Get("https://api.ipify.org")
	if err != nil {
		return err
	}
	defer req.Body.Close()
	ip, err2 := ioutil.ReadAll(req.Body)
	if err2 != nil {
		return err2
	}
	opt := CreateAgentOptions{
		Hostname:  agent.Hostname,
		Localtime: localtime(),
		Version:   version,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		IP:        strings.TrimSpace(string(ip)),
	}
	if len(agent.Labels) > 0 {
		opt.Labels = agent.Labels
	}
	if len(interfaces) > 0 {
		opt.Interfaces = interfaces
	}
	status, err := agent.Client.Post("run/agents/", opt, &agent)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("Failed to create agent http code: %d", status)
	}
	var MasterIP []string
	err = agent.checkSuperNetwork(MasterIP)
	if err != nil {
		return err
	}
	logger.Printf("Agent %s registered with hostname %s (agent version %s)\n", agent.ID, agent.Hostname, version)
	return nil
}

func (agent *Agent) infinitePing() {
	ticker := time.NewTicker(3 * time.Second).C
	for {
		select {
		case <-ticker:
			err := agent.Ping()
			if err != nil {
				logger.Errorf("Cannot ping %+v", err)
				logger.Info("Trying to register lost agent")
				err := agent.Register()
				if err != nil {
					logger.Errorf("Unable to register lost agent: %+v", err)
					continue
				}
				errInitTracked := agent.initTrackedContainers()
				if errInitTracked != nil {
					logger.Errorf("Unable to init tracked containers: %+v", errInitTracked)
					continue
				}
				errSync := agent.forceSyncTrackedDockerContainers()
				if err != nil {
					logger.Errorf("Unable to sync containers: %+v", errSync)
					continue
				}
				info, err := agent.DockerClient.Info(context.Background())
				if err != nil {
					logger.Errorf("Cannot fetch docker info %+v", err)
					continue
				} else {
					agent.syncDockerInfo(info)
				}
				streamer.destroy()
				streamer = NewStreamer(fmt.Sprintf("aid:%s", agent.ID), true)
				go streamer.run()
			}
		}
	}
}

// Ping agent
func (agent *Agent) Ping() error {
	pingURI := fmt.Sprintf("run/agents/%s/ping/", agent.ID)
	flatRunningTasksList := make([]string, 0, len(runningTasksList))
	for k := range runningTasksList {
		flatRunningTasksList = append(flatRunningTasksList, k)
	}
	opts := PingAgentOptions{
		Metrics:       metrics,
		RunningTasks:  flatRunningTasksList,
		Localtime:     localtime(),
		NumGoroutines: runtime.NumGoroutine(),
	}
	for t := range runningTerminalsList {
		opts.RunningTerminals = append(opts.RunningTerminals, t.Tid)
	}
	status, err := agent.Client.Post(pingURI, opts, nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("ping to %s returns %d codes", pingURI, status)
	}
	nbGoroutines := runtime.NumGoroutine()
	// logger.Debugf("Ping OK (%d running task%s, %d running terminal%s, %d goroutine%s)", len(opts.RunningTasks), pluralize(len(opts.RunningTasks)), len(opts.RunningTerminals), pluralize(len(opts.RunningTerminals)), nbGoroutines, pluralize(nbGoroutines))
	logger.WithFields(logrus.Fields{
		"tasks":         len(opts.RunningTasks),
		"terminals":     len(opts.RunningTerminals),
		"goroutines":    nbGoroutines,
		"containerLogs": fmt.Sprintf("%d/1024", len(streamer.msg)),
		"containers":    len(trackedContainers),
	}).Debug("Ping OK")
	return nil
}

func (agent *Agent) ackTask(task *Task, taskACK *TaskACK) error {
	ackURI := fmt.Sprintf("run/agents/%s/tasks/unack/%s/", agent.ID, task.ID)
	_, err := agent.Client.Delete(ackURI, &taskACK, nil)
	if err != nil {
		logger.Error(err)
		return err
	}
	return nil
}

func (agent *Agent) infinitePullTasks() {
	for {
		tasks, err := agent.pullTasks()
		if err != nil {
			logger.Errorf("Cannot pull tasks: %+v", err)
		}
		if len(tasks) > 0 {
			tasksID := []string{}
			for _, t := range tasks {
				tasksID = append(tasksID, t.ID)
			}
			logger.Infof("Got %d new tasks %s", len(tasks), strings.Join(tasksID, ", "))
		}
		for _, task := range tasks {
			task.cancelCh = make(chan bool)
			if task.NeedACK {
				lock.RLock()
				runningTasksList[task.ID] = task
				lock.RUnlock()
			}
			go func(t *Task) {
				logger.Infof("running task %s", t.ID)
				err := t.Do()
				if err != nil {
					logger.Errorf("Unable to do task %s: %s", t.ID, err)
				}
				logger.Infof("task %s done!", t.ID)
			}(task)
		}
		time.Sleep(3 * time.Second)
	}
}

func (agent *Agent) pullTasks() ([]*Task, error) {
	tasksURI := fmt.Sprintf("run/agents/%s/tasks/ready/?requeue=false&size=50", agent.ID)
	tasks := []*Task{}
	err := agent.Client.Get(tasksURI, &tasks)
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

type versionUpdate struct {
	Version         string `json:"version"`
	Os              string `json:"os"`
	Arch            string `json:"arch"`
	ArchiveURL      string `json:"archive_url"`
	ArchiveChecksum string `json:"archive_checksum"`
	BinaryChecksum  string `json:"binary_checksum"`
	BinarySignature string `json:"binary_signature"`
}

func (agent *Agent) getLatestVersion() (*versionUpdate, error) {
	versionURI := fmt.Sprintf("run/agent-version/latest/?os=%s&arch=%s", runtime.GOOS, runtime.GOARCH)
	v := &versionUpdate{}
	err := agent.Client.Get(versionURI, &v)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (agent *Agent) update() error {
	v, err := agent.getLatestVersion()
	if err != nil {
		return err
	}
	if v.Version == version {
		logger.Infof("Agent version %s is up to date.", version)
		return nil
	}
	logger.Infof("Preparing update from %s to %s", version, v.Version)
	checksum, err := hex.DecodeString(v.BinaryChecksum)
	if err != nil {
		return err
	}
	signature, err := hex.DecodeString(v.BinarySignature)
	if err != nil {
		return err
	}
	logger.Infof("Downloading %s", v.ArchiveURL)
	response, err := httpClient.Get(v.ArchiveURL)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	logger.Infof("Uncompressing run-agent %v update", v.Version)
	gzReader, err := gzip.NewReader(response.Body)
	if err != nil {
		return err
	}
	binaryReader, binaryWriter := io.Pipe()
	defer binaryReader.Close()
	defer binaryWriter.Close()
	tarReader := tar.NewReader(gzReader)
	for {
		header, errTarReader := tarReader.Next()
		if errTarReader == io.EOF {
			break
		}
		if errTarReader != nil {
			return err
		}
		if header.Name == "run-agent" {
			go func() {
				if _, errIoTar := io.Copy(binaryWriter, tarReader); errIoTar != nil {
					logger.Fatalf("ExtractTarGz: Copy() failed: %s", errIoTar.Error())
				}
				defer binaryWriter.Close()
			}()
			break
		}
	}
	logger.Infof("Applying update")
	opts := update.Options{
		Checksum:  checksum,
		Signature: signature,
	}
	publicKey := `
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEA+UzJD+more/0adp0/IKYGl9OgO1
A5t0SQ22qx1j3A6ozKZpNGTQ8JZCudWza3vuZ9RcjsBfbBZVmWZwqDMYbQ==
-----END PUBLIC KEY-----`
	err = opts.SetPublicKeyPEM([]byte(publicKey))
	if err != nil {
		return fmt.Errorf("Could not parse public key: %v", err)
	}
	errUpdate := update.Apply(binaryReader, opts)
	if errUpdate != nil {
		return errUpdate
	}
	logger.Infof("Update done from %s to version %s", version, v.Version)
	shutdown(249)
	return nil
}
