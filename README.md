# FranTransport

FranTransport is an experimental, transport-agnostic P2P overlay networking
framework. V1 proves one architectural boundary: applications use stable
FranTransport APIs while Tailcat remains a replaceable data-plane detail.

The current vertical slice includes:

- stable `NodeID` and `PeerID` domain types;
- a small stream-oriented `Transport` contract;
- reusable contract tests and an in-memory mock transport;
- a Tailcat v0.7.0 adapter with direct-path discovery and DERP fallback handled
  entirely by Tailcat;
- an HTTP/JSON + SQLite control plane with expiring, single-use enrollment
  tokens, stable overlay allocation, authenticated discovery, heartbeats, and a
  server-rendered node dashboard;
- `frantransportd`, which persists its local identity, enrolls, accepts peer
  streams, echoes framed test payloads, and exposes a loopback status/test API.

This is a prototype. It is not a production VPN, authorization system, or
multi-tenant service.

## Requirements

- Go 1.27.1 or newer (required by the pinned Tailcat v0.7.0 dependency)

## Build and test

```sh
go build ./cmd/frantransport-controlplane ./cmd/frantransportd
go test ./...
go test -race ./...
```

The public-DERP Tailcat contract test is opt-in because it performs real
network I/O:

```sh
FRANTRANSPORT_TAILCAT_INTEGRATION=1 go test ./pkg/transport/tailcat -run TestContract -v
```

## Local demo

Start the control plane:

```sh
go run ./cmd/frantransport-controlplane -db ./frantransport.db
```

In two separate shells, issue two short-lived enrollment tokens. Each command
prints the secret once; do not paste it into logs or commit it:

```sh
go run ./cmd/frantransport-controlplane -db ./frantransport.db -issue-token 10m
```

Start two nodes with separate identity files and local API ports:

```sh
FRANTRANSPORT_ENROLLMENT_TOKEN='<token-a>' go run ./cmd/frantransportd \
  -name alpha -identity /tmp/frantransport-alpha/identity.json \
  -local-listen 127.0.0.1:7777

FRANTRANSPORT_ENROLLMENT_TOKEN='<token-b>' go run ./cmd/frantransportd \
  -name beta -identity /tmp/frantransport-beta/identity.json \
  -local-listen 127.0.0.1:7778
```

Open `http://127.0.0.1:8080/` to see both nodes. Read beta's `peer_id` from
`http://127.0.0.1:7778/status`, then ask alpha to exchange a payload:

```sh
curl -X POST http://127.0.0.1:7777/connect \
  -H 'Content-Type: application/json' \
  -d '{"peer_id":"node_REPLACE_ME","payload":"aGVsbG8="}'
```

JSON byte slices use base64, so the example payload is `hello`. The control
plane only distributes peer metadata; the payload goes over the Tailcat-backed
peer stream. After first enrollment, the node token is loaded from the
mode-0600 identity file and the enrollment environment variable is no longer
needed.

See [architecture](docs/architecture.md) and the [implementation plan](docs/implementation-plan.md).
