# ADR 0003: Isolate pinned Tailcat behind composition

Status: accepted

FranTransport pins `github.com/tailscale/tailcat` v0.7.0 and confines all use of
its unstable Go API to `pkg/transport/tailcat`. The adapter translates generic
peer metadata, owns client/server lifecycle, suppresses secret-prone backend
logging, and exposes only standard `net.Conn` values to the transport layer.

FranTransport does not fork Tailcat. Upstream API changes should require adapter
changes and contract-test validation, not public API changes.
