# V1 implementation plan

1. **Tailcat API inspection — complete.** The pinned v0.7.0 API provides
   `Server.Listen` and `Client.DialTCPPort`, so V1 uses a bidirectional stream
   abstraction. Tailcat requires Go 1.27.1 and explicitly makes no API stability
   promise.
2. **Transport boundary, mock, and contract tests — complete.** Payload exchange
   and deterministic lifecycle behavior run without public network access.
3. **Control-plane skeleton and enrollment — complete.** HTTP/JSON, SQLite,
   single-use expiring tokens, authenticated peer discovery, stable allocation,
   heartbeat, offline updates, and a server-rendered dashboard are implemented.
4. **Node daemon and public client API — complete.** Local identity persistence,
   enrollment, discovery, framed payload exchange, status, and graceful offline
   notification are implemented.
5. **Tailcat adapter — compiled and contract-ready.** Upstream types are isolated
   in `pkg/transport/tailcat`; the same transport contract test is available as
   an opt-in public-DERP integration test.
6. **Two-host validation — pending environment validation.** Run the documented
   demo on two independently networked hosts and record whether Tailcat reports a
   direct or relayed path once a generic path signal is available upstream.

Every step keeps the repository buildable. Features outside the stated V1 scope
remain intentionally absent.
