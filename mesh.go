package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	weave_plugin "github.com/pikacloud/run-agent/plugins/weave"
)

// AgentPeerConnection describe a peer connection to or from the agent weave router
type AgentPeerConnection struct {
	IP               string `json:"ip"`
	Port             int    `json:"port"`
	State            string `json:"state"`
	Info             string `json:"info"`
	Blacklisted      bool   `json:"blacklisted"`
	Outbound         bool   `json:"outbound"`
	ForgetInProgress bool   `json:"forget_in_progress"`
	Name             string `json:"name"`
}

func (c AgentPeerConnection) key() string {
	return fmt.Sprintf("%s:%d", c.IP, c.Port)
}

func (c AgentPeerConnection) getPeer(status *weave_plugin.Status) *weave_plugin.PeerStatus {
	for _, peer := range status.Router.MeshStatus.Peers {
		for _, connection := range peer.Connections {
			if connection.Address == c.key() {
				return &peer
			}
		}
	}
	return nil
}

func (agent *Agent) readMeshInfo() (map[string]*AgentPeerConnection, error) {
	peers := make(map[string]*AgentPeerConnection)
	status, err := agent.weave.Status()
	if err != nil {
		return nil, err
	}
	for _, c := range status.Router.Connections {
		s := strings.Split(c.Address, ":")
		if len(s) != 2 {
			continue
		}
		port, err := strconv.Atoi(s[1])
		if err != nil {
			continue
		}
		ip := s[0]
		connection := &AgentPeerConnection{
			IP:       ip,
			Port:     port,
			State:    c.State,
			Info:     c.Info,
			Outbound: c.Outbound,
		}
		if peer := connection.getPeer(status); peer != nil {
			connection.Name = peer.Name
		}
		peers[c.Address] = connection
	}
	return peers, nil
}

// connectPeers connects agent to other peers
func (agent *Agent) connectPeers(ips []string) error {
	for _, ip := range ips {
		if err := agent.weave.Connect(ip); err != nil {
			return err
		}
	}
	return nil
}

// connectPeers connects agent to other peers
func (agent *Agent) forgetPeers(ips []string) error {
	for _, ip := range ips {
		if err := agent.weave.Forget(ip); err != nil {
			return err
		}
	}
	maxTries := 30
	t := 0
	var has string
	for {
		has = ""
		for _, ip := range ips {
			for _, peer := range agent.peers {
				if peer.IP == ip {
					has = ip
					break
				}
			}
		}
		time.Sleep(1 * time.Second)
		if has == "" {
			break
		}
		if t > maxTries {
			return fmt.Errorf("Cannot forget peer %s", has)
		}
		t++
	}
	return nil
}
