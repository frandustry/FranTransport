// Package abstraction contains transport-independent FranTransport domain types.
package abstraction

import "time"

type NodeID string
type PeerID string

type ConnectionState string

const (
	StateDisconnected ConnectionState = "disconnected"
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateDegraded     ConnectionState = "degraded"
	StateFailed       ConnectionState = "failed"
)

type Path string

const (
	PathUnknown Path = "unknown"
	PathDirect  Path = "direct"
	PathRelay   Path = "relay"
)

// Endpoint is opaque to callers but interpreted by the selected transport.
type Endpoint struct {
	Transport string            `json:"transport"`
	Metadata  map[string]string `json:"metadata"`
}

type Peer struct {
	ID        PeerID   `json:"peer_id"`
	NodeID    NodeID   `json:"node_id"`
	Name      string   `json:"name"`
	OverlayIP string   `json:"overlay_ip"`
	Endpoint  Endpoint `json:"endpoint"`
}

type ConnectionInfo struct {
	PeerID PeerID          `json:"peer_id"`
	State  ConnectionState `json:"state"`
	Path   Path            `json:"path"`
	RTT    time.Duration   `json:"rtt,omitempty"`
}
