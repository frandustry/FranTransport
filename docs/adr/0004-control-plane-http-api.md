# ADR 0004: Use HTTP/JSON and SQLite for V1 coordination

Status: accepted

V1 needs enrollment, stable identity/IP allocation, peer metadata exchange, and
liveness, not a distributed control system. A standard-library HTTP/JSON server
and one SQLite database keep deployment and inspection simple.

Enrollment tokens are random, expiring, single-use values. Node API calls use a
separate random bearer credential. The control plane has no payload endpoint.
OAuth/OIDC, ACLs, multi-tenancy, gRPC, and production authorization remain out
of scope.
