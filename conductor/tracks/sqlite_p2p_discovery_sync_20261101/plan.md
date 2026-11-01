# Implementation Plan - P2P Peer Discovery and Sync Protocol for sqlite-p2p Nodes

## Phase 1: Peer Discovery & Membership Management
Establish peer discovery mechanisms, membership state tracking, and periodic liveness heartbeats.

- [x] Task: Implement Peer Registry and Liveness Model (fecffba)
    - [x] Subtask: Write failing unit tests for Peer Registry (add, remove, list active, liveness eviction)
    - [x] Subtask: Implement `shared/p2p/peer_registry.go` with thread-safe peer state management
    - [x] Subtask: Verify test coverage >80%
- [~] Task: Implement Hybrid Peer Discovery (mDNS & Static Seeds)
    - [ ] Subtask: Write failing unit tests for static seed resolution and mock mDNS advertisement/discovery
    - [ ] Subtask: Implement `shared/p2p/discovery.go` with static seed pinging and local network discovery
    - [ ] Subtask: Verify discovery tests pass with >80% coverage
- [ ] Task: Conductor - User Manual Verification 'Peer Discovery & Membership Management' (Protocol in workflow.md)

## Phase 2: Change Tracking, Conflict Resolution & Sync Protocol
Implement change log capture, vector/logical timestamps, delta packaging, and deterministic conflict resolution.

- [ ] Task: Implement Change Log & Lamport Clock Mutation Tracker
    - [ ] Subtask: Write failing tests for mutation tracking and Lamport clock increments on transaction/registration writes
    - [ ] Subtask: Implement `shared/p2p/changelog.go` and integrate mutation recording with `shared/storage/sqlite_repository.go`
    - [ ] Subtask: Verify changelog tracking tests pass with >80% coverage
- [ ] Task: Implement P2P Sync Protocol & Conflict Resolver
    - [ ] Subtask: Write failing tests for push/pull delta sync and Last-Write-Wins (LWW) conflict resolution
    - [ ] Subtask: Implement `shared/p2p/sync_service.go` handling sync handshakes, delta requests, and atomic apply
    - [ ] Subtask: Verify sync engine tests pass with >80% coverage
- [ ] Task: Conductor - User Manual Verification 'Change Tracking, Conflict Resolution & Sync Protocol' (Protocol in workflow.md)

## Phase 3: Transport Security, Status Observability & End-to-End Integration
Secure P2P channels with Ed25519 participant keys, expose status endpoints, and write multi-node sync integration tests.

- [ ] Task: Ed25519 Peer Authentication & Secure Handshake
    - [ ] Subtask: Write failing tests verifying signed handshake verification using ONDC participant Ed25519 keys
    - [ ] Subtask: Implement secure peer handshake verifier in `shared/p2p/auth.go`
    - [ ] Subtask: Verify authentication tests pass with >80% coverage
- [ ] Task: P2P HTTP Status Handler & Integration Test
    - [ ] Subtask: Implement `/p2p/status` HTTP handler reporting active peers, sync state, and clock vector
    - [ ] Subtask: Write multi-node end-to-end integration test (`test/p2p_sync_e2e_test.go`) simulating 3 interconnected nodes syncing transaction data
    - [ ] Subtask: Verify integration test suite passes
- [ ] Task: Conductor - User Manual Verification 'Transport Security, Status Observability & End-to-End Integration' (Protocol in workflow.md)
