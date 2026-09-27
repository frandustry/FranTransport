package frantransport_test

import (
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/frandustry/FranTransport/pkg/abstraction"
	"github.com/frandustry/FranTransport/pkg/frantransport"
	"github.com/frandustry/FranTransport/pkg/transport/mock"
)

type resolver struct{ peer abstraction.Peer }

func (r resolver) ResolvePeer(context.Context, abstraction.PeerID) (abstraction.Peer, error) {
	return r.peer, nil
}

func TestClientConnectAndExchangePayload(t *testing.T) {
	n := mock.NewNetwork()
	a, _ := n.NewTransport("a")
	b, _ := n.NewTransport("b")
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		conn, err := b.Accept(ctx)
		if err != nil {
			return
		}
		defer conn.Close()
		var header [4]byte
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return
		}
		payload := make([]byte, binary.BigEndian.Uint32(header[:]))
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}
		if _, err := conn.Write(header[:]); err != nil {
			return
		}
		_, _ = conn.Write(payload)
	}()
	client, err := frantransport.New(frantransport.Config{Resolver: resolver{peer: abstraction.Peer{ID: "b", Endpoint: b.Endpoint()}}, Transport: a})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := client.Connect(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	payload := []byte{0, 1, 2, 3, 255}
	if err := conn.Send(ctx, payload); err != nil {
		t.Fatal(err)
	}
	got, err := conn.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %v, want %v", got, payload)
	}
	if info := conn.Info(); info.State != abstraction.StateConnected || info.Path != abstraction.PathUnknown {
		t.Fatalf("unexpected info: %+v", info)
	}
}
