# Implementation Plan: Unified Self-Sovereign Node & AutoCert Daemon (`cmd/ondc-node`)

## Phase 1: Configuration, Core Server Engine & Route Multiplexing [checkpoint: cbbaf99]
- [x] Task: Write unit tests for unified node config resolution and router multiplexing (`shared/node/config_test.go`, `shared/node/router_test.go`) [1c681b4]
    - [x] Test precedence: CLI flags > Environment variables > Defaults
    - [x] Test routing table: `/on_subscribe`, `/ondc-site-verification.html`, `/bap/*`, `/bpp/*`, `/p2p/*`, `/healthz`, `/status`
- [x] Task: Implement unified configuration and routing multiplexer (`shared/node/config.go`, `shared/node/router.go`) [1c681b4]
    - [x] Implement `LoadNodeConfig` using standard library `flag` and environment resolution (no `spf13`)
    - [x] Implement `NewNodeMux` combining onboarding, buyer, seller, and P2P HTTP endpoints
- [x] Task: Conductor - User Manual Verification 'Phase 1: Configuration, Core Server Engine & Route Multiplexing' (Protocol in workflow.md) [cbbaf99]

## Phase 2: Network Listeners, Dual-Mode TLS (autocert) & Embedded P2P Bridge [checkpoint: bc771fd]
- [x] Task: Write unit tests for HTTP/HTTPS listener factory and embedded bridge orchestration (`shared/node/server_test.go`) [cb5d156]
    - [x] Test HTTP server initialization in tunnel/local mode
    - [x] Test AutoCert manager initialization, HTTP-01 challenge handler, and localhost fallback
    - [x] Test embedded `BridgeDaemon` start/stop lifecycle coordination
- [x] Task: Implement dual-mode listener and embedded bridge manager (`shared/node/server.go`) [cb5d156]
    - [x] Implement `RunServer` supporting plain HTTP and `autocert.Manager` TLS
    - [x] Embed and orchestrate `shared/p2p.BridgeDaemon` background replication
    - [x] Coordinate unified graceful shutdown (SIGINT/SIGTERM) with timeout draining
- [x] Task: Conductor - User Manual Verification 'Phase 2: Network Listeners, Dual-Mode TLS (autocert) & Embedded P2P Bridge' (Protocol in workflow.md) [bc771fd]

## Phase 3: Unified Binary Entrypoint (`cmd/ondc-node`), Bazel Rules & E2E Testing
- [x] Task: Write end-to-end integration tests for unified server (`test/unified_node_e2e_test.go`) [e5975f4]
    - [x] Test running unified server, hitting onboarding `/ondc-site-verification.html`, buyer/seller endpoints, and verifying background P2P replication
    - [x] Test graceful shutdown of all listeners and SQLite connections
- [ ] Task: Implement CLI entrypoint (`cmd/ondc-node/main.go`), Bazel target (`cmd/ondc-node/BUILD.bazel`), and documentation
    - [ ] Implement `cmd/ondc-node/main.go` and `BUILD.bazel`
    - [ ] Update `docs/ARCHITECTURE.md` and `docs/DEVOPS.md` with unified node single-binary deployment guides (Cloudflare Tunnel vs Standalone VM with Let's Encrypt)
- [ ] Task: Conductor - User Manual Verification 'Phase 3: Unified Binary Entrypoint (cmd/ondc-node), Bazel Rules & E2E Testing' (Protocol in workflow.md)
