package controlplane

import (
	"context"
	"errors"
	"time"

	"github.com/frandustry/FranTransport/pkg/abstraction"
)

var ErrUnauthorized = errors.New("unauthorized")

type Node struct {
	ID                   abstraction.NodeID          `json:"node_id"`
	PeerID               abstraction.PeerID          `json:"peer_id"`
	Name                 string                      `json:"name"`
	OverlayIP            string                      `json:"overlay_ip"`
	PublicKey            string                      `json:"public_key"`
	PublicKeyFingerprint string                      `json:"public_key_fingerprint"`
	Endpoint             abstraction.Endpoint        `json:"endpoint"`
	State                abstraction.ConnectionState `json:"state"`
	Online               bool                        `json:"online"`
	LastSeen             time.Time                   `json:"last_seen"`
}

type Store interface {
	IssueEnrollmentToken(context.Context, time.Duration) (string, error)
	Enroll(context.Context, EnrollRequest) (Node, string, error)
	Authenticate(context.Context, string) (abstraction.NodeID, error)
	Heartbeat(context.Context, abstraction.NodeID, HeartbeatRequest) error
	SetOffline(context.Context, abstraction.NodeID) error
	ListNodes(context.Context) ([]Node, error)
	Close() error
}

type EnrollRequest struct {
	Token                string               `json:"token"`
	Name                 string               `json:"name"`
	PublicKey            string               `json:"public_key"`
	PublicKeyFingerprint string               `json:"public_key_fingerprint"`
	Endpoint             abstraction.Endpoint `json:"endpoint"`
}

type HeartbeatRequest struct {
	Endpoint abstraction.Endpoint        `json:"endpoint"`
	State    abstraction.ConnectionState `json:"state"`
}

type EnrollResponse struct {
	Node      Node   `json:"node"`
	NodeToken string `json:"node_token"`
}
