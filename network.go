package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	docker_types "github.com/docker/docker/api/types"
	fernet "github.com/fernet/fernet-go"
	"github.com/pikacloud/gopikacloud"
	weave_plugin "github.com/pikacloud/run-agent/plugins/weave"
)

// getNetInterfaces describes the function who pushes all active interfaces from host to connect to network
func (agent *Agent) getNetInterfaces() ([]string, error) {
	var SysInt []string
	var DockInt []string
	var Ret []string
	Cards, err := net.InterfaceAddrs()
	if err != nil {
		return Ret, err
	}
	for _, card := range Cards {
		if ipnet, ok := card.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				SysInt = append(SysInt, card.String())
			}
		}
	}

	ctx := context.Background()
	nl, err := agent.DockerClient.NetworkList(ctx, docker_types.NetworkListOptions{})
	if err != nil {
		return Ret, err
	}
	for _, net := range nl {
		if len(net.IPAM.Config) > 0 {
			DockInt = append(DockInt, net.IPAM.Config[0].Subnet)
		}
	}
	for _, scard := range SysInt {
		_, ipv4Net, err2 := net.ParseCIDR(scard)
		if err2 != nil {
			return Ret, err2
		}
		test := true
		for _, dcard := range DockInt {
			if ipv4Net.String() == dcard {
				test = false
			}
		}
		if test == true {
			Ret = append(Ret, scard)
		}
	}
	return Ret, nil
}

// detachNetworks detaches networks from a container
func (agent *Agent) detachNetworks(containerID string, subnetworks []*subNetwork) error {
	for _, subnetwork := range subnetworks {
		ips, err := agent.weave.Detach(containerID, subnetwork.Cidr)
		if err != nil {
			return err
		}
		for _, ip := range ips {
			agent.removeContainerDNSRecord(containerID, ip)
		}
	}
	return nil
}

type connections struct {
	address  string
	outbound bool
	state    string
	info     string
}

type ppeers struct {
	name        string
	nickname    string
	connections []connections
}

type superNetworkStatus struct {
	peers []ppeers
}

type subNetworkEntry struct {
	IP          string   `json:"ip"`
	ContainerID string   `json:"container_id"`
	DNSRecords  []string `json:"dns_records"`
}

type subNetwork struct {
	DNSDomain string             `json:"domain"`
	Cidr      string             `json:"cidr"`
	Entries   []*subNetworkEntry `json:"entries"`
}

type superNetwork struct {
	config      gopikacloud.SuperNetwork
	subNetworks map[string]*subNetwork
}

func (sn *superNetwork) decodeKey(p string) string {
	bytesKey := make([]byte, 32)
	copy(bytesKey, []byte(p))
	key := base64.StdEncoding.EncodeToString(bytesKey)
	k := fernet.MustDecodeKeys(key)
	password := fernet.VerifyAndDecrypt([]byte(sn.config.Key), 60*time.Second, k)
	return string(password)
}

// handleSuperNetwork checks that weave is running with a correct configuration
func (agent *Agent) handleSuperNetwork() error {
	superNetworkOpts := gopikacloud.SuperNetwork{
		Cidr:      agentSuperNetworkCIDR,
		DNSDomain: agentSuperNetworkDomain,
	}
	config, err := pikacloudClient.CreateSuperNetwork(superNetworkOpts, agent.ID)
	if err != nil {
		return fmt.Errorf("Unable to retrieve agent SuperNetwork config: %s", err)
	}
	agent.superNetwork = &superNetwork{
		config: config,
	}
	var debug bool
	loglevel := os.Getenv("LOG_LEVEL")
	switch strings.ToUpper(loglevel) {
	case "DEBUG":
		debug = true
	}

	agent.weave = weave_plugin.NewWeave(config.DNSDomain, config.Cidr, agent.superNetwork.decodeKey(agent.ID), debug)
	agent.peers = make(map[string]*AgentPeerConnection)
	status, err := agent.weave.Auto()
	if err != nil {
		return err
	}
	agent.PeerID = status.Router.MeshStatus.Name
	return nil
}

func (agent *Agent) updateContainerDNSRecord(containerID string, ip string, dnsEntry string) error {
	err := agent.weave.DNSAdd(containerID, ip, dnsEntry)
	if err != nil {
		return err
	}
	return nil
}

func (agent *Agent) removeContainerDNSRecord(containerID string, ip string) error {
	err := agent.weave.DNSRemove(containerID, ip)
	if err != nil {
		return err
	}
	return nil
}

func diffInSubNetworks(s1 []*subNetwork, s2 []*subNetwork) bool {
	if len(s1) != len(s2) {
		return true
	}
	if s1 == nil || s2 == nil {
		return true
	}
	return false
}

func (agent *Agent) syncNetworkRouterStatus() error {
	if agent.weave == nil {
		return fmt.Errorf("Network router subsystem not yet initialized")
	}
	// sync mesh status if necessary
	weavePeers, err := agent.readMeshInfo()
	if err != nil {
		return err
	}
	lock.Lock()
	for key, peer := range weavePeers {
		agent.peers[key] = peer
	}
	lock.Unlock()
	currentInterfaces, err := agent.getNetInterfaces()
	if err != nil {
		return err
	}
	lock.Lock()
	agent.interfaces = currentInterfaces
	lock.Unlock()
	// sync container network info if necessary
	var containers []*AgentContainer
	for _, trackedContainer := range trackedContainers {
		currentSubnetworks, err := agent.readContainerNetworkInfo(trackedContainer.ID)
		if err != nil {
			return err
		}
		// compare current networks with tracked ones
		if diffInSubNetworks(trackedContainer.Networks, currentSubnetworks) {
			containers = append(containers, trackedContainer)
			trackedContainer.Networks = currentSubnetworks
		}

	}
	if len(containers) > 0 {
		agent.syncDockerContainers(containers)
	}

	return nil
}

// NetworkRouterStatus describe the agent networking status
type NetworkRouterStatus struct {
	Interfaces  []string
	WeaveStatus *weave_plugin.Status
}

func (agent *Agent) infiniteSyncNetworkRouterStatus() {
	logger.Info("Network router watchdog started")
	defer func() {
		logger.Info("Network router watchdog exited")
	}()
	for {
		if err := agent.syncNetworkRouterStatus(); err != nil {
			logger.Debug(err)
			if err := agent.handleSuperNetwork(); err != nil {
				logger.Debug(err)
			}
		}
		time.Sleep(3 * time.Second)
	}
}

// attachNetworks attaches networks to a container
func (agent *Agent) attachNetworks(containerID string, subnetworks []*subNetwork, dnsName string) ([]string, error) {
	ips := []string{}
	for _, subnetwork := range subnetworks {
		ip, err := agent.weave.Attach(containerID, subnetwork.Cidr)
		if err != nil {
			return nil, fmt.Errorf("Cannot attach container %s to network %s: %s", containerID, subnetwork.Cidr, err)
		}
		logger.Debugf("Container %s attached to network %s.", containerID, subnetwork.Cidr)
		agent.addTrackedContainerNetworkInfo(containerID, ip, subnetwork)
		ips = append(ips, ip)
		dnsEntry := fmt.Sprintf("%s.%s%s", dnsName, subnetwork.DNSDomain, agent.superNetwork.config.DNSDomain)
		agent.updateContainerDNSRecord(containerID, ip, dnsEntry)
		dnsEntryCID := fmt.Sprintf("%s.%s%s", containerID[:12], subnetwork.DNSDomain, agent.superNetwork.config.DNSDomain)
		agent.updateContainerDNSRecord(containerID, ip, dnsEntryCID)
	}
	return ips, nil
}

func (agent *Agent) readContainerNetworkInfo(containerID string) ([]*subNetwork, error) {
	ps, err := agent.weave.PS(containerID)
	if err != nil {
		return nil, err
	}
	status, err := agent.weave.Status()
	if err != nil {
		return nil, err
	}

	subnetworks := []*subNetwork{}
	for _, p := range ps {
		for _, ip := range p.IP {
			ipv4Addr, ipv4Net, err := net.ParseCIDR(ip)
			if err != nil {
				return nil, err
			}
			dnsRecords := []string{}
			for _, d := range status.DNS.Entries {
				if d.Tombstone > 0 {
					continue
				}
				if d.Address == ipv4Addr.String() {
					dnsRecords = append(dnsRecords, d.Hostname)
				}
			}
			subnetwork := &subNetwork{
				Cidr: ipv4Net.String(),
			}
			subnetwork.Entries = append(subnetwork.Entries, &subNetworkEntry{
				IP:          ipv4Addr.String(),
				ContainerID: containerID,
				DNSRecords:  dnsRecords,
			})
			subnetworks = append(subnetworks, subnetwork)

		}

	}
	return subnetworks, nil
}

func (agent *Agent) addTrackedContainerNetworkInfo(containerID string, ip string, subnetwork *subNetwork) error {
	trackedContainer := trackedContainers[containerID]
	if trackedContainer == nil {
		return fmt.Errorf("Unknown tracked container %s", containerID)
	}
	exists := false
	for _, network := range trackedContainer.Networks {
		if network.Cidr == subnetwork.Cidr {
			for _, entry := range subnetwork.Entries {
				if entry.IP == ip {
					exists = true
				}
			}
		}
	}
	subnetwork.Entries = append(subnetwork.Entries, &subNetworkEntry{
		IP:          ip,
		ContainerID: containerID,
	})
	if !exists {
		trackedContainer.Networks = append(trackedContainer.Networks, subnetwork)
	}
	return nil
}

// NetworkConnectOpts describes the connect to peers structure
type NetworkConnectOpts struct {
	IPs []string `json:"ips"`
}

// NetworkAttachOpts describes the options required to attach a container to
// a network
type NetworkAttachOpts struct {
	Cidr        string `json:"cidr"`
	DNSDomain   string `json:"dns_domain"`
	ContainerID string `json:"container_id"`
	DNSName     string `json:"dns_name"`
}

// Network handles networking operations
func (step *TaskStep) Network() error {
	switch step.Method {
	case "connect":
		var connectOpts = NetworkConnectOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &connectOpts)
		if err != nil {
			return fmt.Errorf("Bad config for network connect: %s (%v)", err, step.PluginConfig)
		}
		healthyIPs := []string{}
		for _, ip := range connectOpts.IPs {
			blacklistedPeer := &AgentPeerConnection{
				IP:          ip,
				Port:        6783,
				Blacklisted: time.Now().Unix(),
				Outbound:    true,
			}
			if peer := agent.peers[blacklistedPeer.key()]; peer != nil {
				if peer.Blacklisted > 0 {
					continue
				}
			}
			if errTCP := agent.weave.TestTCPConnection(ip); errTCP != nil {
				logger.Debugf("Cannot connect to peer %s: %v", ip, errTCP)
				blacklistedPeer.Info = errTCP.Error()
				blacklistedPeer.State = "blacklisted"
				agent.peers[blacklistedPeer.key()] = blacklistedPeer
				if err := agent.syncNetworkRouterStatus(); err != nil {
					return err
				}
			} else {
				healthyIPs = append(healthyIPs, ip)
			}
		}
		if len(healthyIPs) == 0 {
			return fmt.Errorf("No remote peers reachables: %s", strings.Join(connectOpts.IPs, ", "))
		}
		err = agent.connectPeers(healthyIPs)
		if err != nil {
			return fmt.Errorf("Failure attempting to add connections to network router: %s", err)
		}
		return nil
	case "forget":
		var forgetOpts = NetworkConnectOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &forgetOpts)
		if err != nil {
			return fmt.Errorf("Bad config for network forget: %s (%v)", err, step.PluginConfig)
		}
		err = agent.forgetPeers(forgetOpts.IPs)
		if err != nil {
			return fmt.Errorf("Cannot forget peers: %s", err)
		}
		return nil
	case "attach":
		var attachOpts = NetworkAttachOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &attachOpts)
		if err != nil {
			return fmt.Errorf("Bad config for network attach: %s (%v)", err, step.PluginConfig)
		}
		subnetwork := &subNetwork{
			Cidr:      attachOpts.Cidr,
			DNSDomain: attachOpts.DNSDomain,
		}
		subnetworks := []*subNetwork{subnetwork}
		_, err = agent.attachNetworks(attachOpts.ContainerID, subnetworks, attachOpts.DNSName)
		if err != nil {
			return fmt.Errorf("Cannot attach container %s to network %s: %s", attachOpts.ContainerID, attachOpts.Cidr, err)
		}
		step.ResultMessage = fmt.Sprintf("Container %s attached to network %s", attachOpts.ContainerID, attachOpts.Cidr)
		agent.syncDockerContainer(attachOpts.ContainerID)
		return nil
	case "detach":
		var detachOpts = NetworkAttachOpts{}
		err := json.Unmarshal([]byte(step.PluginConfig), &detachOpts)
		if err != nil {
			return fmt.Errorf("Bad config for network detach: %s (%v)", err, step.PluginConfig)
		}
		subnetwork := &subNetwork{
			Cidr: detachOpts.Cidr,
		}
		subnetworks := []*subNetwork{subnetwork}
		err = agent.detachNetworks(detachOpts.ContainerID, subnetworks)
		if err != nil {
			return fmt.Errorf("Cannot attach container %s to network %s: %s", detachOpts.ContainerID, detachOpts.Cidr, err)
		}
		step.ResultMessage = fmt.Sprintf("Container %s detached from network %s", detachOpts.ContainerID, detachOpts.Cidr)
		agent.syncDockerContainer(detachOpts.ContainerID)
		return nil
	case "reset":
		err := agent.weave.Destroy()
		if err != nil {
			return fmt.Errorf("Cannot reset network router: %s", err)
		}
		step.ResultMessage = fmt.Sprintf("Network router resetted")
		return nil
	default:
		return fmt.Errorf("Unknown step method %s", step.Method)
	}
}
