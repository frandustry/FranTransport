// Package frantransport is the stable application-facing API.
package frantransport

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/frandustry/FranTransport/pkg/abstraction"
	"github.com/frandustry/FranTransport/pkg/transport"
)

const maxPayload = 16 << 20

type PeerResolver interface {
	ResolvePeer(context.Context, abstraction.PeerID) (abstraction.Peer, error)
}

type Config struct {
	Resolver  PeerResolver
	Transport transport.Transport
}

type Client struct {
	resolver  PeerResolver
	transport transport.Transport
}

func New(cfg Config) (*Client, error) {
	if cfg.Resolver == nil {
		return nil, errors.New("frantransport: resolver is required")
	}
	if cfg.Transport == nil {
		return nil, errors.New("frantransport: transport is required")
	}
	return &Client{resolver: cfg.Resolver, transport: cfg.Transport}, nil
}

func (c *Client) Connect(ctx context.Context, peerID abstraction.PeerID) (*Connection, error) {
	peer, err := c.resolver.ResolvePeer(ctx, peerID)
	if err != nil {
		return nil, fmt.Errorf("resolve peer %s: %w", peerID, err)
	}
	conn, err := c.transport.Dial(ctx, peer)
	if err != nil {
		return nil, fmt.Errorf("connect peer %s: %w", peerID, err)
	}
	return &Connection{peerID: peerID, conn: conn, reader: bufio.NewReader(conn), state: abstraction.StateConnected}, nil
}

func (c *Client) Close() error { return c.transport.Close() }

type Connection struct {
	peerID abstraction.PeerID
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
	state  abstraction.ConnectionState
}

func (c *Connection) Info() abstraction.ConnectionInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return abstraction.ConnectionInfo{PeerID: c.peerID, State: c.state, Path: abstraction.PathUnknown}
}

func (c *Connection) Send(ctx context.Context, payload []byte) error {
	if len(payload) > maxPayload {
		return fmt.Errorf("payload exceeds %d bytes", maxPayload)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(deadline)
		defer c.conn.SetWriteDeadline(timeZero)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeAll(c.conn, header[:]); err != nil {
		c.fail()
		return err
	}
	if err := writeAll(c.conn, payload); err != nil {
		c.fail()
		return err
	}
	return nil
}

func (c *Connection) Receive(ctx context.Context) ([]byte, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetReadDeadline(deadline)
		defer c.conn.SetReadDeadline(timeZero)
	}
	var header [4]byte
	if _, err := io.ReadFull(c.reader, header[:]); err != nil {
		c.fail()
		return nil, err
	}
	n := binary.BigEndian.Uint32(header[:])
	if n > maxPayload {
		c.fail()
		return nil, fmt.Errorf("incoming payload exceeds %d bytes", maxPayload)
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(c.reader, p); err != nil {
		c.fail()
		return nil, err
	}
	return p, nil
}

func (c *Connection) Close() error {
	c.mu.Lock()
	c.state = abstraction.StateDisconnected
	c.mu.Unlock()
	return c.conn.Close()
}

func (c *Connection) fail() { c.mu.Lock(); c.state = abstraction.StateFailed; c.mu.Unlock() }

var timeZero time.Time

func writeAll(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		p = p[n:]
	}
	return nil
}
