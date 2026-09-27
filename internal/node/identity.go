package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/frandustry/FranTransport/pkg/abstraction"
)

type Identity struct {
	PrivateKey string             `json:"private_key"`
	NodeID     abstraction.NodeID `json:"node_id,omitempty"`
	PeerID     abstraction.PeerID `json:"peer_id,omitempty"`
	OverlayIP  string             `json:"overlay_ip,omitempty"`
	NodeToken  string             `json:"node_token,omitempty"`
}

func LoadOrCreateIdentity(path string) (*Identity, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		var id Identity
		if err := json.Unmarshal(b, &id); err != nil {
			return nil, fmt.Errorf("decode identity: %w", err)
		}
		if _, err := id.private(); err != nil {
			return nil, err
		}
		return &id, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	id := &Identity{PrivateKey: base64.RawStdEncoding.EncodeToString(priv)}
	if err := id.Save(path); err != nil {
		return nil, err
	}
	return id, nil
}
func (i *Identity) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func (i *Identity) private() (ed25519.PrivateKey, error) {
	b, err := base64.RawStdEncoding.DecodeString(i.PrivateKey)
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid local identity private key")
	}
	return ed25519.PrivateKey(b), nil
}
func (i *Identity) PublicKey() (string, error) {
	p, err := i.private()
	if err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(p.Public().(ed25519.PublicKey)), nil
}
func (i *Identity) Fingerprint() (string, error) {
	p, err := i.PublicKey()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(p))
	return "SHA256:" + hex.EncodeToString(sum[:8]), nil
}
