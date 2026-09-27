// Package contract contains reusable black-box tests for Transport implementations.
package contract

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/frandustry/FranTransport/pkg/abstraction"
	"github.com/frandustry/FranTransport/pkg/transport"
)

type Factory func(t *testing.T) (transport.Transport, transport.Transport)

func TestTransport(t *testing.T, factory Factory) {
	t.Helper()
	a, b := factory(t)
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	payload := []byte("frantransport-contract-\x00-payload")
	received := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := b.Accept(ctx)
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		got := make([]byte, len(payload))
		_, err = io.ReadFull(conn, got)
		if err != nil {
			errCh <- err
			return
		}
		received <- got
	}()

	conn, err := a.Dial(ctx, abstraction.Peer{ID: "b", Endpoint: b.Endpoint()})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = conn.Close()
	select {
	case err := <-errCh:
		t.Fatal(err)
	case got := <-received:
		if string(got) != string(payload) {
			t.Fatalf("payload mismatch: %q", got)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err = a.Dial(ctx, abstraction.Peer{ID: "b", Endpoint: b.Endpoint()})
	if !errors.Is(err, transport.ErrPeerUnavailable) {
		t.Fatalf("Dial closed peer error = %v", err)
	}
}
