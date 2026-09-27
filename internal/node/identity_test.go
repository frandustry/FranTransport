package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdentityPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "identity.json")
	first, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	pub1, err := first.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	first.NodeID = "node_stable"
	first.OverlayIP = "fd7a:115c:a1e0::1"
	if err := first.Save(path); err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	pub2, err := second.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if pub1 != pub2 || second.NodeID != "node_stable" || second.OverlayIP != "fd7a:115c:a1e0::1" {
		t.Fatalf("identity changed: %#v", second)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("identity mode = %o", info.Mode().Perm())
	}
}
