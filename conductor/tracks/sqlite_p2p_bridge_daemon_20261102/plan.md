# Implementation Plan: Standalone SQLite-P2P Auto-Replication Bridge Service

## Phase 1: Configuration & Bridge Daemon Core Engine [checkpoint: ab3df38]
- [x] Task: Create `BridgeConfig` struct with flag and environment variable resolution (`shared/p2p/daemon_config.go`) [9b73618]
    - [x] Write unit tests for configuration resolution priority (CLI flags > ENV > Defaults) in `shared/p2p/daemon_config_test.go`
    - [x] Implement `LoadBridgeConfig(args []string, lookupEnv func(string) (string, bool))` supporting flags and env vars without spf13/cobra
- [x] Task: Implement `BridgeDaemon` engine with background loop & metrics (`shared/p2p/daemon.go`) [602b4d2]
    - [x] Write unit tests for `BridgeDaemon` lifecycle, sync step execution, and error handling in `shared/p2p/daemon_test.go`
    - [x] Implement `BridgeDaemon` struct with `Start(ctx context.Context)`, `Stop()`, `SyncOnce(ctx context.Context)`, and stats tracking
- [x] Task: Implement HTTP Health & Status server (`shared/p2p/daemon_http.go`) [aeffaf5]
    - [x] Write unit tests for `/healthz` and `/status` in `shared/p2p/daemon_http_test.go`
    - [x] Implement HTTP handler routes for health checks and daemon metrics
- [x] Task: Conductor - User Manual Verification 'Phase 1: Configuration & Bridge Daemon Core Engine' (Protocol in workflow.md) [ab3df38]

## Phase 2: Standalone CLI Binary & Bazel Target [checkpoint: da03d6c]
- [x] Task: Implement `cmd/ondc-p2p-bridge/main.go` entrypoint [c40f203]
    - [x] Handcrafted CLI argument parser using standard library `flag` and OS signal trap (SIGINT/SIGTERM)
    - [x] Wire daemon start, HTTP server launch, and clean shutdown handling
- [x] Task: Create Bazel build target `//cmd/ondc-p2p-bridge` in `cmd/ondc-p2p-bridge/BUILD.bazel` [3eddd8f]
    - [x] Define `go_binary` rule and verify `bazel build //cmd/ondc-p2p-bridge` and `go build ./cmd/ondc-p2p-bridge`
- [x] Task: Conductor - User Manual Verification 'Phase 2: Standalone CLI Binary & Bazel Target' (Protocol in workflow.md) [da03d6c]

## Phase 3: End-to-End Integration Testing & Documentation
- [ ] Task: Write end-to-end integration test (`test/p2p_bridge_daemon_e2e_test.go`)
    - [ ] Spin up daemon against two test SQLite databases, write transactions to ONDC DB, verify automatic propagation to `crm_store`, write mutations to `crm_store`, verify automatic convergence in ONDC DB, and test graceful shutdown
- [ ] Task: Update architecture and DevOps documentation
    - [ ] Update `docs/ARCHITECTURE.md` and `docs/DEVOPS.md` with binary usage, systemd/Docker deployment, and configuration reference
- [ ] Task: Conductor - User Manual Verification 'Phase 3: End-to-End Integration Testing & Documentation' (Protocol in workflow.md)
