// Package mock provides an in-memory Transport for deterministic tests.
package mock

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/frandustry/FranTransport/pkg/abstraction"
	"github.com/frandustry/FranTransport/pkg/transport"
)

type Network struct {
	mu    sync.RWMutex
	nodes map[string]*Transport
}

func NewNetwork() *Network { return &Network{nodes: make(map[string]*Transport)} }

type Transport struct {
	id       string
	network  *Network
	incoming chan net.Conn
	done     chan struct{}
	once     sync.Once
}

func (n *Network) NewTransport(id string) (*Transport, error) {
	if id == "" {
		return nil, fmt.Errorf("mock transport: empty id")
	}
	t := &Transport{id: id, network: n, incoming: make(chan net.Conn), done: make(chan struct{})}
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, exists := n.nodes[id]; exists {
		return nil, fmt.Errorf("mock transport: duplicate id %q", id)
	}
	n.nodes[id] = t
	return t, nil
}

func (t *Transport) Name() string { return "mock" }
func (t *Transport) Endpoint() abstraction.Endpoint {
	return abstraction.Endpoint{Transport: t.Name(), Metadata: map[string]string{"id": t.id}}
}

func (t *Transport) Dial(ctx context.Context, peer abstraction.Peer) (net.Conn, error) {
	if peer.Endpoint.Transport != t.Name() {
		return nil, fmt.Errorf("%w: expected mock", transport.ErrInvalidPeerMetadata)
	}
	id := peer.Endpoint.Metadata["id"]
	t.network.mu.RLock()
	dst := t.network.nodes[id]
	t.network.mu.RUnlock()
	if dst == nil {
		return nil, fmt.Errorf("%w: %s", transport.ErrPeerUnavailable, peer.ID)
	}
	local, remote := net.Pipe()
	select {
	case dst.incoming <- remote:
		return local, nil
	case <-ctx.Done():
		_ = local.Close()
		_ = remote.Close()
		return nil, ctx.Err()
	case <-dst.done:
		_ = local.Close()
		_ = remote.Close()
		return nil, transport.ErrPeerUnavailable
	case <-t.done:
		_ = local.Close()
		_ = remote.Close()
		return nil, transport.ErrClosed
	}
}

func (t *Transport) Accept(ctx context.Context) (net.Conn, error) {
	select {
	case c := <-t.incoming:
		return c, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.done:
		return nil, transport.ErrClosed
	}
}

func (t *Transport) Close() error {
	t.once.Do(func() {
		close(t.done)
		t.network.mu.Lock()
		delete(t.network.nodes, t.id)
		t.network.mu.Unlock()
	})
	return nil
}
