package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	docker_client "github.com/moby/moby/client"
	"github.com/pikacloud/gopikacloud"
	weave_plugin "github.com/pikacloud/run-agent/plugins/weave"
)

// CreateAgentOptions represents the agent Create() options
type CreateAgentOptions struct {
	Hostname   string      `json:"hostname"`
	Labels     []string    `json:"labels,omitempty"`
	Localtime  int         `json:"localtime"`
	Version    string      `json:"version"`
	OS         string      `json:"os"`
	Arch       string      `json:"arch"`
	DockerInfo *dockerInfo `json:"docker_info"`
}

// PingAgentOptions represents the agent Ping() options
type PingAgentOptions struct {
	Metrics          *Metrics                        `json:"metrics,omitempty"`
	RunningTasks     []string                        `json:"running_tasks,omitempty"`
	RunningTerminals []string                        `json:"running_terminals,omitempty"`
	Localtime        int                             `json:"localtime"`
	NumGoroutines    int                             `json:"num_goroutines"`
	Interfaces       []string                        `json:"interfaces"`
	Peers            map[string]*AgentPeerConnection `json:"peers"`
}

// Agent describes the agent
type Agent struct {
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
	chSyncPeers           chan string
	startNetworking       bool
	superNetwork          *superNetwork
	peers                 map[string]*AgentPeerConnection
	interfaces            []string
	weave                 *weave_plugin.Weave
	PeerID                string `json:"peer_id"`
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
func NewAgent(hostname string, labels []string, startNetworking bool) *Agent {
	return &Agent{
		Hostname:              hostname,
		DockerClient:          NewDockerClient(""),
		Labels:                labels,
		chRegisterContainer:   make(chan string),
		chDeregisterContainer: make(chan string),
		chSyncContainer:       make(chan string),
		chSyncPeers:           make(chan string),
		startNetworking:       startNetworking,
		peers:                 make(map[string]*AgentPeerConnection),
	}
}

// Register an agent
func (agent *Agent) Register() error {
	opt := CreateAgentOptions{
		Hostname:   agent.Hostname,
		Localtime:  localtime(),
		Version:    version,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		DockerInfo: &dockerInfo{},
	}
	if len(agent.Labels) > 0 {
		opt.Labels = agent.Labels
	}

	if agent.DockerClient != nil {
		info, err := agent.DockerClient.Info(context.Background())
		if err == nil {
			opt.DockerInfo.KernelVersion = info.KernelVersion
			opt.DockerInfo.ServerVersion = info.ServerVersion
		}
	}
	status, err := pikacloudClient.Post("run/agents/", opt, &agent)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("Failed to create agent http code: %d", status)
	}
	if agent.startNetworking {
		logger.Debug("Checking network router...")
		err := agent.handleSuperNetwork()
		if err != nil {
			return fmt.Errorf("Unable to check network router: %s", err)
		}
		logger.Debug("Network router is operational")
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
		Interfaces:    agent.interfaces,
		Peers:         agent.peers,
	}
	for t := range runningTerminalsList {
		opts.RunningTerminals = append(opts.RunningTerminals, t.Tid)
	}
	status, err := pikacloudClient.Post(pingURI, opts, nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("ping to %s returns %d codes", pingURI, status)
	}
	nbGoroutines := runtime.NumGoroutine()
	logger.WithFields(logrus.Fields{
		"tasks":         len(opts.RunningTasks),
		"terminals":     len(opts.RunningTerminals),
		"goroutines":    nbGoroutines,
		"containerLogs": fmt.Sprintf("%d/1024", len(streamer.msg)),
		"containers":    len(trackedContainers),
	}).Debug("Ping OK")
	return nil
}
