package controlplane

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/frandustry/FranTransport/pkg/abstraction"
)

type Server struct {
	Store            Store
	HeartbeatTimeout time.Duration
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /api/v1/enroll", s.enroll)
	mux.HandleFunc("GET /api/v1/peers", s.auth(s.peers))
	mux.HandleFunc("POST /api/v1/heartbeat", s.auth(s.heartbeat))
	mux.HandleFunc("POST /api/v1/offline", s.auth(s.offline))
	mux.HandleFunc("GET /", s.index)
	return securityHeaders(mux)
}

type nodeHandler func(http.ResponseWriter, *http.Request, abstraction.NodeID)

func (s *Server) auth(next nodeHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		id, err := s.Store.Authenticate(r.Context(), strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid bearer token")
			return
		}
		next(w, r, id)
	}
}

func (s *Server) enroll(w http.ResponseWriter, r *http.Request) {
	var req EnrollRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	node, token, err := s.Store.Enroll(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "invalid, expired, or used enrollment token")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, EnrollResponse{Node: node, NodeToken: token})
}

func (s *Server) peers(w http.ResponseWriter, r *http.Request, self abstraction.NodeID) {
	nodes, err := s.Store.ListNodes(r.Context())
	if err != nil {
		writeError(w, 500, "list nodes failed")
		return
	}
	peers := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if n.ID != self {
			peers = append(peers, s.withFreshOnline(n))
		}
	}
	writeJSON(w, 200, map[string]any{"peers": peers})
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request, id abstraction.NodeID) {
	var req HeartbeatRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err := s.Store.Heartbeat(r.Context(), id, req); err != nil {
		writeError(w, 500, "heartbeat failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) offline(w http.ResponseWriter, r *http.Request, id abstraction.NodeID) {
	if err := s.Store.SetOffline(r.Context(), id); err != nil {
		writeError(w, 500, "offline update failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) withFreshOnline(n Node) Node {
	timeout := s.HeartbeatTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	n.Online = n.Online && time.Since(n.LastSeen) <= timeout
	if !n.Online {
		n.State = abstraction.StateDisconnected
	}
	return n
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	nodes, err := s.Store.ListNodes(r.Context())
	if err != nil {
		http.Error(w, "list nodes failed", 500)
		return
	}
	for i := range nodes {
		nodes[i] = s.withFreshOnline(nodes[i])
		nodes[i].PublicKey = ""
		nodes[i].Endpoint = abstraction.Endpoint{}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = indexTemplate.Execute(w, map[string]any{"Nodes": nodes})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>FranTransport</title>
<style>body{font:15px system-ui;margin:2rem;background:#0b1020;color:#e8edf8}main{max-width:1100px;margin:auto}table{width:100%;border-collapse:collapse;background:#141b2d}th,td{padding:.8rem;text-align:left;border-bottom:1px solid #2a3550}.online{color:#68d391}.offline{color:#fc8181}code{font-size:.82em}</style></head>
<body><main><h1>FranTransport</h1><p>Control-plane node inventory. Peer payloads never traverse this service.</p><table><thead><tr><th>Name</th><th>NodeID</th><th>Overlay IP</th><th>Status</th><th>Last seen</th><th>Transport state</th><th>Public-key fingerprint</th></tr></thead><tbody>
{{range .Nodes}}<tr><td>{{.Name}}</td><td><code>{{.ID}}</code></td><td><code>{{.OverlayIP}}</code></td><td class="{{if .Online}}online{{else}}offline{{end}}">{{if .Online}}online{{else}}offline{{end}}</td><td>{{.LastSeen.Format "2006-01-02 15:04:05Z07:00"}}</td><td>{{.State}}</td><td><code>{{.PublicKeyFingerprint}}</code></td></tr>{{else}}<tr><td colspan="7">No enrolled nodes.</td></tr>{{end}}
</tbody></table></main></body></html>`))
