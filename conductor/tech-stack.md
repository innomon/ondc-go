# Technology Stack

## Core Language & Runtime
- **Language:** Go (1.20+)
- **Standard Toolchain:** `go` CLI, `go.mod`, standard library `net/http`, `context`, `crypto/ed25519`
- **Build System:** Bazel (`MODULE.bazel`, `BUILD.bazel`) for hermetic, multi-service compilation and container image generation

## Storage & Database Layer
- **Persistence & Synchronization:** [`sqlite-p2p`](https://github.com/innomon/sqlite-p2p)
- **Engine:** Embedded SQLite with peer-to-peer data replication
- **P2P Discovery & Sync Protocol:**
  - Hybrid peer discovery (mDNS local broadcast + static seed probing via `/p2p/ping`)
  - Thread-safe `PeerRegistry` with heartbeat latency metrics and TTL-based stale peer eviction
  - Incremental change-log tracking with monotonic Lamport logical clocks on `transactions` and `registrations` tables
  - Bidirectional push/pull delta replication streaming change batches
  - Deterministic Last-Write-Wins (LWW) conflict resolution with node ID tie-breaking
  - Cryptographic peer mutual authentication and signed requests using ONDC participant Ed25519 keystore keys
  - Observability HTTP endpoint `/p2p/status` reporting node role, logical clock, total changes, and active peers
- **Use Cases:** Transaction lifecycle state, order tracking, message deduplication, and audit logs without external cloud database dependencies

## Networking, APIs & Exposure
- **Protocol:** HTTP REST with JSON payloads adhering to ONDC Retail Protocol v1.2.0 / OpenAPI v1.0.31
- **RPC & Serialization:** Protocol Buffers & gRPC (`google.golang.org/protobuf`, `google.golang.org/grpc`)
- **Unified Self-Sovereign Node:** `cmd/ondc-node` running onboarding, buyer, seller, and P2P bridge subsystems from a single binary
- **Automated SSL/TLS (ACME):** `golang.org/x/crypto/acme/autocert` for standalone VM direct-to-internet deployments with automated Let's Encrypt certificates (ports 443 HTTPS and 80 HTTP-01 challenge redirect with localhost fallback)
- **Local Deployment & Public Ingress:** Cloudflare Tunnel (`cloudflared`) for sovereign local deployments, enabling secure public HTTPS callbacks and ONDC registry site verification without port forwarding or static IPs
- **Container / Mesh (Optional / Production):** Docker, Kubernetes manifests, Istio Service Mesh

## Cryptography & Security
- **Signature & Verification:** Ed25519 digital signatures with Blake2b payload hashing for ONDC Authorization headers
- **Key Exchange & Encryption:** X25519 encryption, Google Tink (`github.com/google/tink/go`)
- **Utilities:** `golang.org/x/crypto`, PKCS#7 padding (`github.com/zenazn/pkcs7pad`)
- **Key Storage:** Local secure key storage / environment variables / secret manager integration

## Validation & Testing
- **Schema Validation:** `github.com/go-playground/validator/v10`
- **Test Framework:** Standard Go `testing` package with table-driven test patterns and JSON fixture verification (`testdata/*.json`)
