package weave

import (
	"net/http"
	"time"
)

// Weave represents the weave instance
type Weave struct {
	Config       *Config
	StartFailure int
	Debug        bool
	HTTPClient   *http.Client
	BaseURL      string
}

// Config represents the expected Weave config
type Config struct {
	DNSDomain    string
	IPAllocRange string
	Password     string
}

// types from various weaveworks project
// avoid to load original linux only code from weaveworks packages

type PeerUID uint64

type PeerShortID uint16

// MACStatus represents the mac statuses
type MACStatus struct {
	Mac      string
	Name     string
	NickName string
	LastSeen time.Time
}

type unicastRouteStatus struct {
	Dest, Via string
}

// BroadcastRouteStatus is the current state of an established broadcast route.
type broadcastRouteStatus struct {
	Source string
	Via    []string
}

// LocalConnectionStatus is the current state of a physical connection to a peer.
type LocalConnectionStatus struct {
	Address  string
	Outbound bool
	State    string
	Info     string
	Attrs    map[string]interface{}
}

// MeshStatus is our current state as a peer, as taken from a router.
// This is designed to be used as diagnostic information.
type MeshStatus struct {
	Protocol           string
	ProtocolMinVersion int
	ProtocolMaxVersion int
	Encryption         bool
	PeerDiscovery      bool
	Name               string
	NickName           string
	Port               int
	Peers              []PeerStatus
	UnicastRoutes      []unicastRouteStatus
	BroadcastRoutes    []broadcastRouteStatus
	Connections        []LocalConnectionStatus
	TerminationCount   int
	Targets            []string
	OverlayDiagnostics interface{}
	TrustedSubnets     []string
}

type NetworkRouterStatus struct {
	*MeshStatus
	Interface    string
	CaptureStats map[string]int
	MACs         []MACStatus
}

type PaxosStatus struct {
	Elector    bool
	KnownNodes int
	Quorum     uint
}

type IpamStatus struct {
	Paxos            *PaxosStatus
	Range            string
	RangeNumIPs      int
	ActiveIPs        int
	DefaultSubnet    string
	Entries          []EntryStatus
	PendingClaims    []ClaimStatus
	PendingAllocates []string
}

type EntryStatus struct {
	Token       string
	Size        uint32
	Peer        string
	Nickname    string
	IsKnownPeer bool
	Version     uint32
}

// Using 32-bit integer to represent IPv4 address
type Address uint32

type CIDR struct {
	Addr      Address
	PrefixLen int
}
type ClaimStatus struct {
	Ident string
	CIDR  CIDR
}

type PSOutput struct {
	ContainerID string
	MAC         string
	IP          []string
}

type NameServerEntryStatus struct {
	Hostname    string
	Origin      string
	ContainerID string
	Address     string
	Version     int
	Tombstone   int64
}

type NameServerStatus struct {
	Domain   string
	Upstream []string
	Address  string
	TTL      uint32
	Entries  []NameServerEntryStatus
}

type ProxyStatus struct {
	Addresses []string
}

type PluginStatus struct {
	Config         PluginConfig
	DriverName     string
	MeshDriverName string `json:"MeshDriverName,omitempty"`
}

type PluginConfig struct {
	Socket            string
	MeshSocket        string
	Enable            bool
	EnableV2          bool
	EnableV2Multicast bool
	DNS               bool
	DefaultSubnet     string
}

type connectionStatus struct {
	Name        string
	NickName    string
	Address     string
	Outbound    bool
	Established bool
}

// PeerStatus is the current state of a peer in the mesh.
type PeerStatus struct {
	Name        string
	NickName    string
	UID         PeerUID
	ShortID     PeerShortID
	Version     uint64
	Connections []connectionStatus
}

// Status represents the weave status
type Status struct {
	Ready   bool
	Version string
	Router  NetworkRouterStatus `json:"Router,omitempty"`
	IPAM    *IpamStatus         `json:"IPAM,omitempty"`
	DNS     *NameServerStatus   `json:"DNS,omitempty"`
	Proxy   *ProxyStatus        `json:"Proxy,omitempty"`
	Plugin  *PluginStatus       `json:"Plugin,omitempty"`
}
