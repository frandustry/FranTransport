package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/frandustry/FranTransport/internal/controlplane"
	"github.com/frandustry/FranTransport/internal/store"
	"github.com/frandustry/FranTransport/pkg/abstraction"
)

func TestEnrollmentPersistsIdentityAndOverlayIP(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	token, err := db.IssueEnrollmentToken(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req := controlplane.EnrollRequest{Token: token, Name: "alpha", PublicKey: "public-a", PublicKeyFingerprint: "fp-a", Endpoint: abstraction.Endpoint{Transport: "mock", Metadata: map[string]string{"id": "a"}}}
	node, nodeToken, err := db.Enroll(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if node.ID == "" || node.PeerID != abstraction.PeerID(node.ID) || node.OverlayIP == "" || nodeToken == "" {
		t.Fatalf("incomplete enrollment: %+v", node)
	}
	if _, _, err := db.Enroll(ctx, req); !errors.Is(err, controlplane.ErrUnauthorized) {
		t.Fatalf("reused enrollment token error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	nodes, err := db.ListNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].ID != node.ID || nodes[0].OverlayIP != node.OverlayIP {
		t.Fatalf("persisted nodes = %+v", nodes)
	}
	if got, err := db.Authenticate(ctx, nodeToken); err != nil || got != node.ID {
		t.Fatalf("authenticate = %q, %v", got, err)
	}
}
