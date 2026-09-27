package node

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/frandustry/FranTransport/internal/controlplane"
	"github.com/frandustry/FranTransport/internal/store"
	"github.com/frandustry/FranTransport/pkg/abstraction"
	"github.com/frandustry/FranTransport/pkg/transport/mock"
)

type handlerTransport struct{ handler http.Handler }

func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	t.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

func TestTwoNodesDiscoverAndExchangePayload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := (&controlplane.Server{Store: db}).Handler()
	httpClient := &http.Client{Transport: handlerTransport{handler: handler}}
	tokenA, err := db.IssueEnrollmentToken(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tokenB, err := db.IssueEnrollmentToken(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	network := mock.NewNetwork()
	transportA, _ := network.NewTransport("a")
	transportB, _ := network.NewTransport("b")
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.json")
	pathB := filepath.Join(dir, "b.json")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dA, err := NewDaemon(DaemonConfig{Name: "alpha", IdentityPath: pathA, EnrollmentToken: tokenA, ControlPlane: &ControlPlaneClient{BaseURL: "http://control.invalid", HTTPClient: httpClient}, Transport: transportA, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	dB, err := NewDaemon(DaemonConfig{Name: "beta", IdentityPath: pathB, EnrollmentToken: tokenB, ControlPlane: &ControlPlaneClient{BaseURL: "http://control.invalid", HTTPClient: httpClient}, Transport: transportB, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 2)
	go func() { errCh <- dA.Run(ctx) }()
	go func() { errCh <- dB.Run(ctx) }()
	idA := waitEnrolled(t, pathA)
	idB := waitEnrolled(t, pathB)
	exchangeCtx, exchangeCancel := context.WithTimeout(ctx, 2*time.Second)
	defer exchangeCancel()
	payload := []byte("control-plane discovery; peer data plane")
	got, info, err := dA.Exchange(exchangeCtx, idB.PeerID, payload)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q", got)
	}
	if info.State != abstraction.StateConnected {
		t.Fatalf("connection = %+v", info)
	}
	cancel()
	for range 2 {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("daemon did not stop")
		}
	}
	nodes, err := db.ListNodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("nodes = %d", len(nodes))
	}
	for _, n := range nodes {
		if n.Online {
			t.Fatalf("node %s still online", n.ID)
		}
	}
	if idA.NodeID == idB.NodeID || idA.OverlayIP == idB.OverlayIP {
		t.Fatal("node identities or overlay IPs are not unique")
	}
}

func waitEnrolled(t *testing.T, path string) *Identity {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		id, err := LoadOrCreateIdentity(path)
		if err == nil && id.NodeID != "" && id.NodeToken != "" {
			return id
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("identity %s was not enrolled", path)
	return nil
}
