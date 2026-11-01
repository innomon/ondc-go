# Technology Stack

## Core Language & Runtime
- **Language:** Go (1.20+)
- **Standard Toolchain:** `go` CLI, `go.mod`, standard library `net/http`, `context`, `crypto/ed25519`
- **Build System:** Bazel (`MODULE.bazel`, `BUILD.bazel`) for hermetic, multi-service compilation and container image generation

## Storage & Database Layer
- **Persistence & Synchronization:** [`sqlite-p2p`](https://github.com/innomon/sqlite-p2p)
- **Engine:** Embedded SQLite with peer-to-peer data replication
- **Use Cases:** Transaction lifecycle state, order tracking, message deduplication, and audit logs without external cloud database dependencies

## Networking, APIs & Exposure
- **Protocol:** HTTP REST with JSON payloads adhering to ONDC Retail Protocol v1.2.0 / OpenAPI v1.0.31
- **RPC & Serialization:** Protocol Buffers & gRPC (`google.golang.org/protobuf`, `google.golang.org/grpc`)
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
