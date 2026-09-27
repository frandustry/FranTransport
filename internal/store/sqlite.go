package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"time"

	"github.com/frandustry/FranTransport/internal/controlplane"
	"github.com/frandustry/FranTransport/pkg/abstraction"
	_ "modernc.org/sqlite"
)

type SQLite struct {
	db  *sql.DB
	now func() time.Time
}

func OpenSQLite(path string) (*SQLite, error) {
	if path != ":memory:" {
		f, err := os.OpenFile(path, os.O_CREATE, 0o600)
		if err != nil {
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// V1 uses a single local SQLite writer. Serializing through one connection
	// avoids transient SQLITE_BUSY failures during simultaneous heartbeats and
	// graceful shutdown updates.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &SQLite{db: db, now: time.Now}
	if err := s.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS enrollment_tokens (
  token_hash TEXT PRIMARY KEY, expires_at INTEGER NOT NULL, used_at INTEGER
);
CREATE TABLE IF NOT EXISTS nodes (
  node_id TEXT PRIMARY KEY, name TEXT NOT NULL, overlay_ip TEXT NOT NULL UNIQUE,
  public_key TEXT NOT NULL UNIQUE, fingerprint TEXT NOT NULL,
  transport TEXT NOT NULL, metadata_json TEXT NOT NULL,
  auth_hash TEXT NOT NULL UNIQUE, state TEXT NOT NULL,
  online INTEGER NOT NULL, last_seen INTEGER NOT NULL
);`)
	return err
}

func (s *SQLite) Close() error { return s.db.Close() }

func (s *SQLite) IssueEnrollmentToken(ctx context.Context, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		return "", errors.New("token TTL must be positive")
	}
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO enrollment_tokens(token_hash, expires_at) VALUES(?, ?)`, hash(token), s.now().Add(ttl).Unix())
	return token, err
}

func (s *SQLite) Enroll(ctx context.Context, req controlplane.EnrollRequest) (controlplane.Node, string, error) {
	if req.Token == "" || req.Name == "" || req.PublicKey == "" || req.Endpoint.Transport == "" {
		return controlplane.Node{}, "", errors.New("token, name, public_key, and endpoint.transport are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return controlplane.Node{}, "", err
	}
	defer tx.Rollback()
	now := s.now()
	res, err := tx.ExecContext(ctx, `UPDATE enrollment_tokens SET used_at=? WHERE token_hash=? AND used_at IS NULL AND expires_at>=?`, now.Unix(), hash(req.Token), now.Unix())
	if err != nil {
		return controlplane.Node{}, "", err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return controlplane.Node{}, "", controlplane.ErrUnauthorized
	}

	var existing controlplane.Node
	err = scanNode(tx.QueryRowContext(ctx, nodeSelect+` WHERE public_key=?`, req.PublicKey), &existing)
	if err == nil {
		return controlplane.Node{}, "", errors.New("identity already enrolled")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return controlplane.Node{}, "", err
	}

	nodeIDRaw, err := randomToken(16)
	if err != nil {
		return controlplane.Node{}, "", err
	}
	nodeID := abstraction.NodeID("node_" + nodeIDRaw)
	nodeToken, err := randomToken(32)
	if err != nil {
		return controlplane.Node{}, "", err
	}
	ip, err := nextOverlayIP(ctx, tx)
	if err != nil {
		return controlplane.Node{}, "", err
	}
	metadata, err := json.Marshal(req.Endpoint.Metadata)
	if err != nil {
		return controlplane.Node{}, "", err
	}
	state := abstraction.StateDisconnected
	_, err = tx.ExecContext(ctx, `INSERT INTO nodes(node_id,name,overlay_ip,public_key,fingerprint,transport,metadata_json,auth_hash,state,online,last_seen) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, nodeID, req.Name, ip, req.PublicKey, req.PublicKeyFingerprint, req.Endpoint.Transport, string(metadata), hash(nodeToken), state, 1, now.Unix())
	if err != nil {
		return controlplane.Node{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return controlplane.Node{}, "", err
	}
	return controlplane.Node{ID: nodeID, PeerID: abstraction.PeerID(nodeID), Name: req.Name, OverlayIP: ip, PublicKey: req.PublicKey, PublicKeyFingerprint: req.PublicKeyFingerprint, Endpoint: req.Endpoint, State: state, Online: true, LastSeen: now}, nodeToken, nil
}

func (s *SQLite) Authenticate(ctx context.Context, token string) (abstraction.NodeID, error) {
	if token == "" {
		return "", controlplane.ErrUnauthorized
	}
	var id abstraction.NodeID
	if err := s.db.QueryRowContext(ctx, `SELECT node_id FROM nodes WHERE auth_hash=?`, hash(token)).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", controlplane.ErrUnauthorized
		}
		return "", err
	}
	return id, nil
}

func (s *SQLite) Heartbeat(ctx context.Context, id abstraction.NodeID, req controlplane.HeartbeatRequest) error {
	metadata, err := json.Marshal(req.Endpoint.Metadata)
	if err != nil {
		return err
	}
	state := req.State
	if state == "" {
		state = abstraction.StateDisconnected
	}
	res, err := s.db.ExecContext(ctx, `UPDATE nodes SET transport=?,metadata_json=?,state=?,online=1,last_seen=? WHERE node_id=?`, req.Endpoint.Transport, string(metadata), state, s.now().Unix(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLite) SetOffline(ctx context.Context, id abstraction.NodeID) error {
	_, err := s.db.ExecContext(ctx, `UPDATE nodes SET online=0,state=?,last_seen=? WHERE node_id=?`, abstraction.StateDisconnected, s.now().Unix(), id)
	return err
}

const nodeSelect = `SELECT node_id,name,overlay_ip,public_key,fingerprint,transport,metadata_json,state,online,last_seen FROM nodes`

func (s *SQLite) ListNodes(ctx context.Context) ([]controlplane.Node, error) {
	rows, err := s.db.QueryContext(ctx, nodeSelect+` ORDER BY name,node_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []controlplane.Node
	for rows.Next() {
		var n controlplane.Node
		if err := scanNode(rows, &n); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanNode(row scanner, n *controlplane.Node) error {
	var metadata string
	var online int
	var lastSeen int64
	if err := row.Scan(&n.ID, &n.Name, &n.OverlayIP, &n.PublicKey, &n.PublicKeyFingerprint, &n.Endpoint.Transport, &metadata, &n.State, &online, &lastSeen); err != nil {
		return err
	}
	n.PeerID = abstraction.PeerID(n.ID)
	n.Online = online != 0
	n.LastSeen = time.Unix(lastSeen, 0).UTC()
	return json.Unmarshal([]byte(metadata), &n.Endpoint.Metadata)
}

func nextOverlayIP(ctx context.Context, tx *sql.Tx) (string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT overlay_ip FROM nodes`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	used := map[string]bool{}
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return "", err
		}
		used[ip] = true
	}
	prefix := netip.MustParsePrefix("fd7a:115c:a1e0::/64")
	base := prefix.Addr().As16()
	for i := uint64(1); i < 1<<24; i++ {
		b := base
		for j := 0; j < 8; j++ {
			b[15-j] = byte(i >> (8 * j))
		}
		ip := netip.AddrFrom16(b).String()
		if !used[ip] {
			return ip, nil
		}
	}
	return "", errors.New("overlay address pool exhausted")
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func hash(v string) string { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }

var _ controlplane.Store = (*SQLite)(nil)
