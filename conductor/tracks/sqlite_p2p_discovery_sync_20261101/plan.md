# Implementation Plan - P2P Peer Discovery and Sync Protocol for sqlite-p2p Nodes

## Phase 1: Peer Discovery & Membership Management [checkpoint: 3e61d5c]
Establish peer discovery mechanisms, membership state tracking, and periodic liveness heartbeats.

- [x] Task: Implement Peer Registry and Liveness Model (fecffba)
    - [x] Subtask: Write failing unit tests for Peer Registry (add, remove, list active, liveness eviction)
    - [x] Subtask: Implement `shared/p2p/peer_registry.go` with thread-safe peer state management
    - [x] Subtask: Verify test coverage >80%
- [x] Task: Implement Hybrid Peer Discovery (mDNS & Static Seeds) (46b26d3)
    - [x] Subtask: Write failing unit tests for static seed resolution and mock mDNS advertisement/discovery
    - [x] Subtask: Implement `shared/p2p/discovery.go` with static seed pinging and local network discovery
    - [x] Subtask: Verify discovery tests pass with >80% coverage
- [x] Task: Conductor - User Manual Verification 'Peer Discovery & Membership Management' (3e61d5c)

## Phase 2: Change Tracking, Conflict Resolution & Sync Protocol [checkpoint: 2f297d7]
Implement change log capture, vector/logical timestamps, delta packaging, and deterministic conflict resolution.

- [x] Task: Implement Change Log & Lamport Clock Mutation Tracker (ff58038)
    - [x] Subtask: Write failing tests for mutation tracking and Lamport clock increments on transaction/registration writes
    - [x] Subtask: Implement `shared/p2p/changelog.go` and integrate mutation recording with `shared/storage/sqlite_repository.go`
    - [x] Subtask: Verify changelog tracking tests pass with >80% coverage
- [x] Task: Implement P2P Sync Protocol & Conflict Resolver (2d5837f)
    - [x] Subtask: Write failing tests for push/pull delta sync and Last-Write-Wins (LWW) conflict resolution
    - [x] Subtask: Implement `shared/p2p/sync_service.go` handling sync handshakes, delta requests, and atomic apply
    - [x] Subtask: Verify sync engine tests pass with >80% coverage
- [x] Task: Conductor - User Manual Verification 'Change Tracking, Conflict Resolution & Sync Protocol' (2f297d7)

## Phase 3: Transport Security, Status Observability & End-to-End Integration
Secure P2P channels with Ed25519 participant keys, expose status endpoints, and write multi-node sync integration tests.

- [x] Task: Ed25519 Peer Authentication & Secure Handshake (1137d7f)
    - [x] Subtask: Write failing tests verifying signed handshake verification using ONDC participant Ed25519 keys
    - [x] Subtask: Implement secure peer handshake verifier in `shared/p2p/auth.go`
    - [x] Subtask: Verify authentication tests pass with >80% coverage
- [x] Task: P2P HTTP Status Handler & Integration Test (14a199f)
    - [x] Subtask: Implement `/p2p/status` HTTP handler reporting active peers, sync state, and clock vector
    - [x] Subtask: Write multi-node end-to-end integration test (`test/p2p_sync_e2e_test.go`) simulating 3 interconnected nodes syncing transaction data
    - [x] Subtask: Verify integration test suite passes
- [ ] Task: Conductor - User Manual Verification 'Transport Security, Status Observability & End-to-End Integration' (Protocol in workflow.md)
