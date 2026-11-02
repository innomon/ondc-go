# ONDC Go Participant Adapter

[![Go Version](https://img.shields.io/badge/Go-1.20%2B-blue.svg)](https://go.dev/)
[![ONDC Protocol](https://img.shields.io/badge/ONDC-Retail%20v1.2.0-orange.svg)](https://ondc.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-green.svg)](/LICENSE)

A sovereign, cloud-agnostic Go implementation of the **Open Network for Digital Commerce (ONDC)** participant adapter. This repository provides buyer and seller platforms, cryptographic onboarding, local ingress tunnels, and a decentralized peer-to-peer (P2P) synchronization layer built on pure-Go SQLite.

---

## Key Features

- **Sovereign & Cloud-Agnostic:** Runs on local bare-metal, containers, edge gateways, or private clouds without mandatory dependencies on proprietary cloud services (e.g. Google Cloud Spanner, Cloud Pub/Sub, or Cloud Secret Manager).
- **Embedded Storage & P2P Replication:** Uses zero-CGO embedded SQLite with an integrated P2P replication engine:
  - Hybrid peer discovery (local mDNS broadcast and static bootstrap seed polling).
  - Incremental mutation tracking with monotonic Lamport logical clocks.
  - Bidirectional push/pull delta replication streaming change batches.
  - Deterministic Last-Write-Wins (LWW) conflict resolution with node ID tie-breaking.
  - Cryptographic peer mutual authentication using ONDC participant Ed25519 signing keys.
  - Live status and observability endpoint at `/p2p/status`.
- **Public Ingress via Cloudflare Tunnel:** Built-in scripts and configuration to deploy sovereign nodes behind Cloudflare Tunnels (`cloudflared`), enabling public HTTPS callbacks and ONDC site verification (`/ondc-site-verification.html`) without port forwarding or static IPs.
- **ONDC Retail Protocol v1.2.0 Compliance:** Standardized payload validation against OpenAPI v1.0.31 with Ed25519 signature generation and Blake2b authorization header verification.
- **Hermetic Toolchain:** Builds and tests cleanly with both the standard Go toolchain (`go test ./...`) and Bazel.

---

## Architecture Overview

```
                      [ ONDC Gateway / Registries ]
                                    │
                                    ▼ (HTTPS Ingress)
                      [ Cloudflare Tunnel / Gateway ]
                                    │
    ┌───────────────────────────────┴──────────────────────────────┐
    │                                                              │
    ▼                                                              ▼
┌─────────────────────────────┐                ┌─────────────────────────────┐
│       Buyer Platform        │                │       Seller Platform       │
│  • BAP Adapter Service      │                │  • BPP Adapter Service      │
│  • Request Action Service   │                │  • Callback Action Service  │
│  • Buyer App API            │                │  • Seller Adapter Service   │
└──────────────┬──────────────┘                └──────────────┬──────────────┘
               │                                              │
               └──────────────────────┬───────────────────────┘
                                      │
                                      ▼
                      ┌───────────────────────────────┐
                      │    Storage & P2P Engine       │
                      │  • SQLite Repository (Pure Go)│
                      │  • Change Log & Lamport Clock │
                      │  • Hybrid Discovery (mDNS/Seed│
                      │  • Ed25519 Peer Mutual Auth   │
                      │  • Push/Pull Sync (/p2p/*)    │
                      └───────────────────────────────┘
```

For an in-depth exploration of the architecture, including how the embedded database relates to the `sqlite-p2p` specification, read the [Architecture Guide](/docs/ARCHITECTURE.md).

---

## Directory Structure

```
.
├── buyer-platform/            # Buyer App Protocol (BAP) microservices
│   ├── bap-adapter-service/   # ONDC protocol adapter for buyer apps
│   ├── bap-api/               # Internal API contracts for buyer applications
│   ├── buyer-app-service/     # Core buyer business logic orchestration
│   └── request-action-service/# Asynchronous ONDC request-action dispatcher
├── seller-platform/           # Buyer Protocol Participant (BPP) microservices
│   ├── bpp-api/               # Internal API contracts for seller applications
│   ├── callback-action-service/# Outbound ONDC callback dispatcher (/on_*)
│   └── seller-adapter-service/# Core seller catalog & fulfillment adapter
├── onboarding/                # Network registration & site verification (/on_subscribe)
├── key-rotation/              # Ed25519 / X25519 key management and rotation service
├── shared/                    # Core libraries shared across all services
│   ├── crypto/                # Cryptographic primitives (Ed25519, X25519, Blake2b)
│   ├── models/                # ONDC Retail v1.2.0 data models and validation
│   ├── p2p/                   # Decentralized peer discovery, Lamport clock & sync protocol
│   ├── storage/               # Pure-Go SQLite repository with change-tracking hooks
│   └── signing-authentication/# HTTP Authorization header signers and verifiers
├── scripts/                   # Local automation and Cloudflare Tunnel helpers
├── test/                      # End-to-end multi-node mesh and ingress tests
└── docs/                      # Architectural documentation and contribution guides
```

---

## Getting Started

### Prerequisites

- **Go:** 1.20 or newer (`go version`)
- **Git:** Standard git toolchain
- **Cloudflared (Optional):** Required only if running public tunnel ingress for live ONDC registry site verification (`/scripts/run_local_tunnel.sh`)

### Installation & Build

Clone the repository and verify the dependencies:

```bash
git clone https://github.com/innomon/ondc-go.git
cd ondc-go
go mod download
```

To compile all services and packages:

```bash
go build ./...
```

---

## Running Tests

Run the full automated test suite (including unit tests, P2P mesh synchronization tests, and cryptographic verification):

```bash
go test -v -race ./...
```

To run only the P2P end-to-end 3-node mesh convergence test:

```bash
go test -v -run TestP2PSyncE2E_MultiNodeMesh ./test/p2p_sync_e2e_test.go
```

To run tests with code coverage analysis:

```bash
go test -coverprofile=coverage.out ./shared/p2p/... ./shared/storage/...
go tool cover -func=coverage.out
```

---

## P2P Synchronization & Storage

ONDC Go ships with an embedded, zero-CGO SQLite storage engine backed by `modernc.org/sqlite`. The replication protocol (`/shared/p2p`) delivers:

- **Incremental Change Log:** Captures all mutations on `transactions` and `registrations` tables.
- **Lamport Clock Ordering:** Monotonic logical timestamps prevent race conditions across distributed nodes.
- **Deterministic Conflict Resolution:** Last-Write-Wins (LWW) with Node ID tie-breaking ensures identical convergent state across all peers.
- **Mutual Peer Authentication:** All P2P synchronization calls (`/p2p/push`, `/p2p/pull`, `/p2p/ping`) require Ed25519 signatures verified against ONDC network credentials.
- **Live Status:** Query node status, logical clock, and connected peers at:
  ```bash
  curl http://localhost:8080/p2p/status
  ```

For details on how the implementation compares to the standalone `sqlite-p2p` engine and when to integrate Hyperswarm DHT networking, see [/docs/ARCHITECTURE.md](/docs/ARCHITECTURE.md#32-pure-go-sqlite-moderncorgsqlite-vs-sqlite-p2p).

---

## Documentation

- [System & Storage Architecture](/docs/ARCHITECTURE.md)
- [DevOps & Operations Guide](/docs/DEVOPS.md)
- [Contributing Guidelines](/docs/contributing.md)
- [Code of Conduct](/docs/code-of-conduct.md)
- [Technology Stack](/conductor/tech-stack.md)

---

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](/LICENSE) file for details.
