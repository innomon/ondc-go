# Implementation Plan: Unified Self-Sovereign Node & AutoCert Daemon (`cmd/ondc-node`)

## Phase 1: Configuration, Core Server Engine & Route Multiplexing
- [ ] Task: Write unit tests for unified node config resolution and router multiplexing (`shared/node/config_test.go`, `shared/node/router_test.go`)
    - [ ] Test precedence: CLI flags > Environment variables > Defaults
    - [ ] Test routing table: `/on_subscribe`, `/ondc-site-verification.html`, `/bap/*`, `/bpp/*`, `/p2p/*`, `/healthz`, `/status`
- [ ] Task: Implement unified configuration and routing multiplexer (`shared/node/config.go`, `shared/node/router.go`)
    - [ ] Implement `LoadNodeConfig` using standard library `flag` and environment resolution (no `spf13`)
    - [ ] Implement `NewNodeMux` combining onboarding, buyer, seller, and P2P HTTP endpoints
- [ ] Task: Conductor - User Manual Verification 'Phase 1: Configuration, Core Server Engine & Route Multiplexing' (Protocol in workflow.md)

## Phase 2: Network Listeners, Dual-Mode TLS (autocert) & Embedded P2P Bridge
- [ ] Task: Write unit tests for HTTP/HTTPS listener factory and embedded bridge orchestration (`shared/node/server_test.go`)
    - [ ] Test HTTP server initialization in tunnel/local mode
    - [ ] Test AutoCert manager initialization, HTTP-01 challenge handler, and localhost fallback
    - [ ] Test embedded `BridgeDaemon` start/stop lifecycle coordination
- [ ] Task: Implement dual-mode listener and embedded bridge manager (`shared/node/server.go`)
    - [ ] Implement `RunServer` supporting plain HTTP and `autocert.Manager` TLS
    - [ ] Embed and orchestrate `shared/p2p.BridgeDaemon` background replication
    - [ ] Coordinate unified graceful shutdown (SIGINT/SIGTERM) with timeout draining
- [ ] Task: Conductor - User Manual Verification 'Phase 2: Network Listeners, Dual-Mode TLS (autocert) & Embedded P2P Bridge' (Protocol in workflow.md)

## Phase 3: Unified Binary Entrypoint (`cmd/ondc-node`), Bazel Rules & E2E Testing
- [ ] Task: Write end-to-end integration tests for unified server (`test/unified_node_e2e_test.go`)
    - [ ] Test running unified server, hitting onboarding `/ondc-site-verification.html`, buyer/seller endpoints, and verifying background P2P replication
    - [ ] Test graceful shutdown of all listeners and SQLite connections
- [ ] Task: Implement CLI entrypoint (`cmd/ondc-node/main.go`), Bazel target (`cmd/ondc-node/BUILD.bazel`), and documentation
    - [ ] Implement `cmd/ondc-node/main.go` and `BUILD.bazel`
    - [ ] Update `docs/ARCHITECTURE.md` and `docs/DEVOPS.md` with unified node single-binary deployment guides (Cloudflare Tunnel vs Standalone VM with Let's Encrypt)
- [ ] Task: Conductor - User Manual Verification 'Phase 3: Unified Binary Entrypoint (cmd/ondc-node), Bazel Rules & E2E Testing' (Protocol in workflow.md)
