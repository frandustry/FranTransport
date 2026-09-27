# Architecture

## Planes and dependency direction

```text
Application
    |
    v
pkg/frantransport          stable Connect/Send/Receive API
    |
    v
pkg/abstraction            NodeID, PeerID, generic state and path
    |
    v
pkg/transport              small stream contract
    ^                ^
    |                |
mock adapter         Tailcat adapter ----> github.com/tailscale/tailcat v0.7.0

frantransportd ----HTTP/JSON----> centralized control plane ----> SQLite
      |
      +============ encrypted peer data plane ==============> peer
```

The control plane stores coordination metadata and never proxies ordinary peer
payloads. The data plane connects peers through the selected transport. Public
FranTransport packages never import Tailcat types.

## Identity model

`NodeID` is the stable control-plane identity and `PeerID` is the stable handle
used by callers. V1 assigns the same opaque value to both while keeping them as
separate types so their roles can diverge later. Neither is a WireGuard key,
Tailcat address, database row number, endpoint, or overlay IP.

Each node generates an Ed25519 identity locally and stores its private material,
assigned IDs, overlay IP, and node credential in a mode-0600 file. Only the
public identity and a short fingerprint are enrolled. SQLite uniquely binds
that public identity to one node record, and the allocated ULA remains stable in
the database and local identity across restarts.

## Enrollment and control API

An administrator creates a cryptographically random, short-lived token directly
through the control-plane command. The database stores only its SHA-256 digest.
Enrollment consumes it exactly once and returns a separate random node bearer
credential; again, only a digest is stored server-side.

Authenticated nodes can list peers, send heartbeats, update their transport
metadata, and mark themselves offline. Anonymous users can only read the
redacted HTML node inventory and health endpoint. The API has no route accepting
peer payloads, which is covered by an integration test.

## Transport abstraction

Tailcat v0.7.0 presents listener/dialer semantics for TCP streams, so the V1
contract contains only `Name`, `Endpoint`, `Dial`, `Accept`, and `Close`. An
endpoint is FranTransport-owned metadata. A transport implementation interprets
its own metadata and converts backend errors to generic sentinel errors where a
stable mapping exists.

The public `frantransport.Client` resolves a `PeerID`, asks the configured
transport to dial, and returns a FranTransport `Connection`. Payload framing,
generic connection state, and cleanup are owned above the adapter. Callers never
observe Tailcat addresses, keys, clients, servers, or path-specific structs.

## Tailcat adapter

The adapter composes, rather than forks, Tailcat. Its server listens on one
application TCP port. Its endpoint metadata contains Tailcat's address and that
port; a remote adapter creates a Tailcat client and dials the port. Tailcat owns
WireGuard encryption, NAT traversal, direct-path upgrades, and DERP fallback.

A default Tailcat address contains a pre-shared key and is therefore a bearer
capability, despite being called connection metadata. FranTransport only returns
it from the authenticated peer API, redacts it from the WebUI, and suppresses
upstream diagnostic logging in the adapter. Closing an outbound session also
closes its Tailcat client.

## Connection lifecycle

1. `frantransportd` starts its transport endpoint.
2. It loads or creates its independent local identity.
3. A new node enrolls once; a known node reuses its stored node credential.
4. Heartbeats publish the current endpoint and generic state.
5. `Client.Connect(ctx, peerID)` resolves an online peer and dials its endpoint.
6. `Send` and `Receive` exchange bounded length-prefixed payloads on the peer
   stream. The control plane is not in this path.
7. Connections and transports close deterministically. Graceful daemon shutdown
   marks the node offline; missed heartbeats also make the WebUI/API treat it as
   offline after the freshness window.

## Package rules

- `pkg/frantransport` may depend on `pkg/abstraction` and `pkg/transport`.
- `pkg/transport` implementations may depend on external backends.
- No package above `pkg/transport/tailcat` may import upstream Tailcat.
- `internal/controlplane` owns HTTP models and handlers; `internal/store` owns
  persistence; `internal/node` composes the public client, transport, and control
  client.
- New transports must first pass the reusable contract test. Backend-specific
  assertions do not belong in high-level behavior tests.
