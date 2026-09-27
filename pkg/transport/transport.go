// Package transport defines the small stream contract implemented by data planes.
package transport

import (
	"context"
	"errors"
	"net"

	"github.com/frandustry/FranTransport/pkg/abstraction"
)

var (
	ErrClosed              = errors.New("transport closed")
	ErrPeerUnavailable     = errors.New("peer unavailable")
	ErrInvalidPeerMetadata = errors.New("invalid peer transport metadata")
)

// Transport is intentionally stream-oriented because Tailcat v0.7.0 exposes
// net.Listener and Dial for TCP streams. Implementations own all path details.
type Transport interface {
	Name() string
	Endpoint() abstraction.Endpoint
	Dial(context.Context, abstraction.Peer) (net.Conn, error)
	Accept(context.Context) (net.Conn, error)
	Close() error
}
