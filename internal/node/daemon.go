package node

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/frandustry/FranTransport/internal/controlplane"
	"github.com/frandustry/FranTransport/pkg/abstraction"
	"github.com/frandustry/FranTransport/pkg/frantransport"
	"github.com/frandustry/FranTransport/pkg/transport"
)

const maxPayload = 16 << 20

type DaemonConfig struct {
	Name            string
	IdentityPath    string
	EnrollmentToken string
	ControlPlane    *ControlPlaneClient
	Transport       transport.Transport
	LocalListen     string
	Logger          *slog.Logger
	HeartbeatEvery  time.Duration
}

type Daemon struct {
	cfg      DaemonConfig
	identity *Identity
	client   *frantransport.Client
	mu       sync.RWMutex
	state    abstraction.ConnectionState
	peers    map[abstraction.PeerID]abstraction.ConnectionInfo
	server   *http.Server
}

func NewDaemon(cfg DaemonConfig) (*Daemon, error) {
	if cfg.ControlPlane == nil || cfg.Transport == nil || cfg.IdentityPath == "" || cfg.Name == "" {
		return nil, errors.New("node: name, identity path, control plane, and transport are required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.HeartbeatEvery <= 0 {
		cfg.HeartbeatEvery = 15 * time.Second
	}
	id, err := LoadOrCreateIdentity(cfg.IdentityPath)
	if err != nil {
		return nil, err
	}
	client, err := frantransport.New(frantransport.Config{Resolver: cfg.ControlPlane, Transport: cfg.Transport})
	if err != nil {
		return nil, err
	}
	return &Daemon{cfg: cfg, identity: id, client: client, state: abstraction.StateDisconnected, peers: make(map[abstraction.PeerID]abstraction.ConnectionInfo)}, nil
}

func (d *Daemon) Run(ctx context.Context) error {
	if err := d.ensureEnrollment(ctx); err != nil {
		return err
	}
	if err := d.heartbeat(ctx); err != nil {
		return fmt.Errorf("initial heartbeat: %w", err)
	}
	errCh := make(chan error, 2)
	go func() { errCh <- d.acceptLoop(ctx) }()
	if d.cfg.LocalListen != "" {
		go func() { errCh <- d.serveLocal(ctx) }()
	}
	ticker := time.NewTicker(d.cfg.HeartbeatEvery)
	defer ticker.Stop()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := d.cfg.ControlPlane.Offline(shutdownCtx); err != nil {
			d.cfg.Logger.Warn("offline update failed", "error", err)
		}
		if d.server != nil {
			_ = d.server.Shutdown(shutdownCtx)
		}
		_ = d.client.Close()
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := d.heartbeat(ctx); err != nil {
				d.cfg.Logger.Warn("heartbeat failed", "error", err)
			}
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, transport.ErrClosed) && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
		}
	}
}

func (d *Daemon) ensureEnrollment(ctx context.Context) error {
	if d.identity.NodeID != "" && d.identity.NodeToken != "" {
		d.cfg.ControlPlane.Token = d.identity.NodeToken
		return nil
	}
	if d.cfg.EnrollmentToken == "" {
		return errors.New("node is not enrolled; set FRANTRANSPORT_ENROLLMENT_TOKEN")
	}
	pub, err := d.identity.PublicKey()
	if err != nil {
		return err
	}
	fingerprint, err := d.identity.Fingerprint()
	if err != nil {
		return err
	}
	resp, err := d.cfg.ControlPlane.Enroll(ctx, controlplane.EnrollRequest{Token: d.cfg.EnrollmentToken, Name: d.cfg.Name, PublicKey: pub, PublicKeyFingerprint: fingerprint, Endpoint: d.cfg.Transport.Endpoint()})
	if err != nil {
		return fmt.Errorf("enroll: %w", err)
	}
	d.identity.NodeID = resp.Node.ID
	d.identity.PeerID = resp.Node.PeerID
	d.identity.OverlayIP = resp.Node.OverlayIP
	d.identity.NodeToken = resp.NodeToken
	if err := d.identity.Save(d.cfg.IdentityPath); err != nil {
		return fmt.Errorf("save enrolled identity: %w", err)
	}
	d.cfg.ControlPlane.Token = resp.NodeToken
	return nil
}

func (d *Daemon) heartbeat(ctx context.Context) error {
	d.mu.RLock()
	state := d.state
	d.mu.RUnlock()
	return d.cfg.ControlPlane.Heartbeat(ctx, controlplane.HeartbeatRequest{Endpoint: d.cfg.Transport.Endpoint(), State: state})
}

func (d *Daemon) acceptLoop(ctx context.Context) error {
	for {
		conn, err := d.cfg.Transport.Accept(ctx)
		if err != nil {
			return err
		}
		go d.echo(conn)
	}
}

func (d *Daemon) echo(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		payload, err := readFrame(reader)
		if err != nil {
			return
		}
		if err := writeFrame(conn, payload); err != nil {
			return
		}
	}
}

func (d *Daemon) serveLocal(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", d.status)
	mux.HandleFunc("POST /connect", d.connect)
	d.server = &http.Server{Addr: d.cfg.LocalListen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	ln, err := net.Listen("tcp", d.cfg.LocalListen)
	if err != nil {
		return fmt.Errorf("local status listen: %w", err)
	}
	d.cfg.Logger.Info("local API listening", "address", ln.Addr().String())
	go func() { <-ctx.Done(); _ = d.server.Close() }()
	return d.server.Serve(ln)
}

func (d *Daemon) status(w http.ResponseWriter, _ *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	peers := make([]abstraction.ConnectionInfo, 0, len(d.peers))
	for _, p := range d.peers {
		peers = append(peers, p)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"node_id": d.identity.NodeID, "peer_id": d.identity.PeerID, "overlay_ip": d.identity.OverlayIP, "state": d.state, "transport": d.cfg.Transport.Name(), "peers": peers})
}

func (d *Daemon) connect(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPayload+4096)
	defer r.Body.Close()
	var req struct {
		PeerID  abstraction.PeerID `json:"peer_id"`
		Payload []byte             `json:"payload"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if req.PeerID == "" {
		http.Error(w, "peer_id is required", 400)
		return
	}
	payload, info, err := d.Exchange(r.Context(), req.PeerID, req.Payload)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"payload": payload, "connection": info})
}

// Exchange connects to a peer, sends one payload, and waits for one reply.
// It is the daemon's minimal V1 data-plane operation and never uses the control
// plane for payload delivery.
func (d *Daemon) Exchange(ctx context.Context, peerID abstraction.PeerID, payload []byte) ([]byte, abstraction.ConnectionInfo, error) {
	d.setPeer(peerID, abstraction.StateConnecting)
	conn, err := d.client.Connect(ctx, peerID)
	if err != nil {
		d.setPeer(peerID, abstraction.StateFailed)
		return nil, abstraction.ConnectionInfo{PeerID: peerID, State: abstraction.StateFailed, Path: abstraction.PathUnknown}, err
	}
	defer conn.Close()
	if err := conn.Send(ctx, payload); err != nil {
		d.setPeer(peerID, abstraction.StateFailed)
		return nil, conn.Info(), err
	}
	reply, err := conn.Receive(ctx)
	if err != nil {
		d.setPeer(peerID, abstraction.StateFailed)
		return nil, conn.Info(), err
	}
	d.setPeer(peerID, abstraction.StateConnected)
	return reply, conn.Info(), nil
}

func (d *Daemon) setPeer(id abstraction.PeerID, state abstraction.ConnectionState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state = state
	d.peers[id] = abstraction.ConnectionInfo{PeerID: id, State: state, Path: abstraction.PathUnknown}
}

func readFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(header[:])
	if n > maxPayload {
		return nil, errors.New("payload too large")
	}
	p := make([]byte, n)
	_, err := io.ReadFull(r, p)
	return p, err
}
func writeFrame(w io.Writer, p []byte) error {
	if len(p) > maxPayload {
		return errors.New("payload too large")
	}
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], uint32(len(p)))
	if err := writeAll(w, h[:]); err != nil {
		return err
	}
	return writeAll(w, p)
}
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
