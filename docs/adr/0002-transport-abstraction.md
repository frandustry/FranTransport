# ADR 0002: Use a small stream listener/dialer contract

Status: accepted

Tailcat v0.7.0 exposes `Server.Listen` and `Client.DialTCPPort`, and the V1 demo
needs reliable arbitrary payload exchange. FranTransport therefore models a
transport as a provider of bidirectional streams with endpoint metadata,
`Dial`, `Accept`, and deterministic `Close`.

Packet, multipath, scoring, and routing abstractions are deferred. They can be
added when a concrete backend or caller requires them without changing the
current public `Connect` and connection API.
