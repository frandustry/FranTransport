package node

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/frandustry/FranTransport/internal/controlplane"
	"github.com/frandustry/FranTransport/pkg/abstraction"
)

type ControlPlaneClient struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func (c *ControlPlaneClient) Enroll(ctx context.Context, req controlplane.EnrollRequest) (controlplane.EnrollResponse, error) {
	var out controlplane.EnrollResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/enroll", req, &out, false)
	return out, err
}
func (c *ControlPlaneClient) Heartbeat(ctx context.Context, req controlplane.HeartbeatRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/heartbeat", req, nil, true)
}
func (c *ControlPlaneClient) Offline(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/v1/offline", struct{}{}, nil, true)
}

func (c *ControlPlaneClient) ResolvePeer(ctx context.Context, id abstraction.PeerID) (abstraction.Peer, error) {
	var out struct {
		Peers []controlplane.Node `json:"peers"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/peers", nil, &out, true); err != nil {
		return abstraction.Peer{}, err
	}
	for _, n := range out.Peers {
		if n.PeerID == id {
			if !n.Online {
				return abstraction.Peer{}, fmt.Errorf("peer %s is offline", id)
			}
			return abstraction.Peer{ID: n.PeerID, NodeID: n.ID, Name: n.Name, OverlayIP: n.OverlayIP, Endpoint: n.Endpoint}, nil
		}
	}
	return abstraction.Peer{}, fmt.Errorf("peer %s not found", id)
}

func (c *ControlPlaneClient) do(ctx context.Context, method, path string, body, out any, auth bool) error {
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return err
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base.String(), reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(limited, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return errors.New(e.Error)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

var _ interface {
	ResolvePeer(context.Context, abstraction.PeerID) (abstraction.Peer, error)
} = (*ControlPlaneClient)(nil)
