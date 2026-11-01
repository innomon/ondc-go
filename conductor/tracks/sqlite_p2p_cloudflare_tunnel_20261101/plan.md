# Implementation Plan: Integrate sqlite-p2p persistence and Cloudflare Tunnel local deployment

## Phase 1: Storage Layer Abstraction and sqlite-p2p Repository
Define the unified storage interface, construct SQLite schema migrations, and implement the `sqlite-p2p` repository driver.

- [x] Task: Define Storage & Repository Interfaces c47dc4c
    - [x] Subtask: Write failing unit tests defining expected repository behaviors for transaction logs and participant registrations
    - [x] Subtask: Implement `storage.Repository` interface with `SaveTransaction`, `GetTransaction`, `SaveRegistration`, and `GetRegistration` methods
    - [x] Subtask: Verify unit tests pass and ensure interface satisfies clean architecture principles
- [x] Task: Implement SQLite Schema & Migrations 1a4b7a3
    - [x] Subtask: Write failing unit tests validating schema initialization, table creation, and constraints
    - [x] Subtask: Implement SQLite DDL statements for `transactions` and `registrations` tables with index definitions
    - [x] Subtask: Verify schema migration tests pass successfully
- [x] Task: Implement sqlite-p2p Repository Driver 12259e2
    - [x] Subtask: Write failing unit tests for sqlite-p2p database connection, CRUD operations, and transaction query helpers
    - [x] Subtask: Integrate `github.com/innomon/sqlite-p2p` client and implement repository methods with thread-safe SQLite connection handling
    - [x] Subtask: Verify tests pass with >80% test coverage
- [x] Task: Conductor - User Manual Verification 'Storage Layer Abstraction and sqlite-p2p Repository' (Protocol in workflow.md)

## Phase 2: Decouple Cloud Spanner & Refactor Platform Adapters
Replace direct Cloud Spanner dependencies with the new sqlite-p2p storage interface across Buyer and Seller platform adapters.

- [x] Task: Refactor Buyer Platform Storage Calls (c027c6a)
    - [x] Subtask: Write failing unit and mock tests for `bap-adapter-service` and `request-action-service` verifying persistence via `storage.Repository`
    - [x] Subtask: Replace Spanner client instances with `storage.Repository` in buyer platform services
    - [x] Subtask: Verify all buyer platform tests pass with >80% coverage
- [~] Task: Refactor Seller Platform Storage Calls
    - [ ] Subtask: Write failing unit and mock tests for `seller-adapter-service` and `callback-action-service` verifying persistence via `storage.Repository`
    - [ ] Subtask: Replace Spanner client instances with `storage.Repository` in seller platform services
    - [ ] Subtask: Verify all seller platform tests pass with >80% coverage
- [ ] Task: Conductor - User Manual Verification 'Decouple Cloud Spanner & Refactor Platform Adapters' (Protocol in workflow.md)

## Phase 3: Cloudflare Tunnel Local Deployment Setup & Verification
Provide configurations, scripts, and documentation for deploying ONDC services locally with Cloudflare Tunnel ingress.

- [ ] Task: Create Cloudflare Tunnel Configuration & Launch Scripts
    - [ ] Subtask: Write failing tests or validation scripts verifying `cloudflared` configuration parsing and port mappings
    - [ ] Subtask: Implement `config/cloudflared/config.yaml.template` and `scripts/run_local_tunnel.sh` mapping BAP, BPP, and Onboarding endpoints
    - [ ] Subtask: Verify configuration validation passes
- [ ] Task: End-to-End Local Ingress & ONDC Challenge Verification Test
    - [ ] Subtask: Write an automated integration test verifying local `/ondc-site-verification.html` and mock callback receipt over configured ingress
    - [ ] Subtask: Implement mock test runner verifying local buyer and seller adapter roundtrip with sqlite-p2p persistence
    - [ ] Subtask: Verify integration test suite passes
- [ ] Task: Conductor - User Manual Verification 'Cloudflare Tunnel Local Deployment Setup & Verification' (Protocol in workflow.md)
