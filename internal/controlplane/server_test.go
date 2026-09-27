package controlplane_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/frandustry/FranTransport/internal/controlplane"
	"github.com/frandustry/FranTransport/internal/store"
	"github.com/frandustry/FranTransport/pkg/abstraction"
)

func TestEnrollmentDiscoveryAndNoPayloadRoute(t *testing.T) {
	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := (&controlplane.Server{Store: db}).Handler()
	ctx := context.Background()
	a, tokenA := enroll(t, ctx, db, handler, "alpha", "pub-a", "a")
	b, _ := enroll(t, ctx, db, handler, "beta", "pub-b", "b")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/peers", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	resp := recorder.Result()
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("peers status = %d", resp.StatusCode)
	}
	var peers struct {
		Peers []controlplane.Node `json:"peers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		t.Fatal(err)
	}
	if len(peers.Peers) != 1 || peers.Peers[0].ID != b.Node.ID {
		t.Fatalf("peers = %+v; self = %s", peers.Peers, a.Node.ID)
	}
	payloadReq := httptest.NewRequest(http.MethodPost, "/api/v1/payload", bytes.NewReader([]byte(`{"payload":"must-not-pass"}`)))
	payloadReq.Header.Set("Authorization", "Bearer "+tokenA)
	payloadRecorder := httptest.NewRecorder()
	handler.ServeHTTP(payloadRecorder, payloadReq)
	payloadResp := payloadRecorder.Result()
	defer payloadResp.Body.Close()
	if payloadResp.StatusCode != http.StatusNotFound {
		t.Fatalf("payload endpoint unexpectedly exists: %s", payloadResp.Status)
	}
}

func enroll(t *testing.T, ctx context.Context, db *store.SQLite, handler http.Handler, name, pub, id string) (controlplane.EnrollResponse, string) {
	t.Helper()
	token, err := db.IssueEnrollmentToken(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(controlplane.EnrollRequest{Token: token, Name: name, PublicKey: pub, PublicKeyFingerprint: "fp-" + id, Endpoint: abstraction.Endpoint{Transport: "mock", Metadata: map[string]string{"id": id}}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	resp := recorder.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("enroll status = %s", resp.Status)
	}
	var out controlplane.EnrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out, out.NodeToken
}
