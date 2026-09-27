// Package tailcat adapts the unstable github.com/tailscale/tailcat API to the
// stable FranTransport stream contract.
package tailcat

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/frandustry/FranTransport/pkg/abstraction"
	"github.com/frandustry/FranTransport/pkg/transport"
	upstream "github.com/tailscale/tailcat"
)

type Config struct {
	DERPMapURL string
}

type Adapter struct {
	server   *upstream.Server
	listener net.Listener
	endpoint abstraction.Endpoint
	once     sync.Once
}

// New starts a Tailcat server and an application stream listener. The
// returned endpoint is a bearer capability and must only be shared with
// authenticated peers.
func New(ctx context.Context, cfg Config) (*Adapter, error) {
	s := &upstream.Server{DERPMapURL: cfg.DERPMapURL, Logf: func(string, ...any) {}}
	ln, err := s.Listen(ctx, "tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("tailcat listen: %w", err)
	}
	_, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		_ = ln.Close()
		_ = s.Close()
		return nil, fmt.Errorf("tailcat listener address: %w", err)
	}
	return &Adapter{
		server:   s,
		listener: ln,
		endpoint: abstraction.Endpoint{Transport: "tailcat", Metadata: map[string]string{
			"address": string(s.TailcatAddr()), "port": portText,
		}},
	}, nil
}

func (a *Adapter) Name() string { return "tailcat" }
func (a *Adapter) Endpoint() abstraction.Endpoint {
	m := make(map[string]string, len(a.endpoint.Metadata))
	for k, v := range a.endpoint.Metadata {
		m[k] = v
	}
	return abstraction.Endpoint{Transport: a.endpoint.Transport, Metadata: m}
}

func (a *Adapter) Accept(ctx context.Context) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() { c, err := a.listener.Accept(); ch <- result{c, err} }()
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, mapError("accept", r.err)
		}
		return r.conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Adapter) Dial(ctx context.Context, peer abstraction.Peer) (net.Conn, error) {
	if peer.Endpoint.Transport != a.Name() {
		return nil, fmt.Errorf("%w: expected tailcat", transport.ErrInvalidPeerMetadata)
	}
	addr, portText := peer.Endpoint.Metadata["address"], peer.Endpoint.Metadata["port"]
	if addr == "" || portText == "" {
		return nil, fmt.Errorf("%w: address or port missing", transport.ErrInvalidPeerMetadata)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return nil, fmt.Errorf("%w: invalid port", transport.ErrInvalidPeerMetadata)
	}
	c := upstream.NewClient(upstream.Addr(addr))
	c.Logf = func(string, ...any) {}
	conn, err := c.DialTCPPort(ctx, uint16(port))
	if err != nil {
		_ = c.Close()
		return nil, mapError("dial", err)
	}
	return &clientConn{Conn: conn, client: c}, nil
}

func (a *Adapter) Close() error {
	var err error
	a.once.Do(func() { err = errors.Join(a.listener.Close(), a.server.Close()) })
	return err
}

type clientConn struct {
	net.Conn
	client *upstream.Client
	once   sync.Once
}

func (c *clientConn) Close() error {
	var err error
	c.once.Do(func() { err = errors.Join(c.Conn.Close(), c.client.Close()) })
	return err
}

func mapError(op string, err error) error {
	if errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("tailcat %s: %w", op, transport.ErrClosed)
	}
	return fmt.Errorf("tailcat %s: %w", op, err)
}
