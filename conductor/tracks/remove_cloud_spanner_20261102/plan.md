# Implementation Plan: Remove All Cloud Spanner References

## Phase 1: Go Source Code & Test Harness Refactoring
- [x] Task: Refactor `shared/clients/transactionclient` to eliminate `SpannerClient` (d5e15b0)
    - [x] Subtask: Write unit tests in `shared/clients/transactionclient` validating transaction storage via `storage.Repository`
    - [x] Subtask: Remove `cloud.google.com/go/spanner` imports and `SpannerClient` struct
    - [x] Subtask: Update `transactionclient.NewClient` to use SQLite repository exclusively
- [x] Task: Refactor `shared/transactiontest` to remove Spanner Admin APIs and emulator hooks (56f6e7f)
    - [x] Subtask: Refactor `shared/transactiontest/transactiontest.go` to provision in-memory/temp SQLite test databases
    - [x] Subtask: Remove Spanner database and instance admin client code and emulator address checks
- [x] Task: Conductor - User Manual Verification 'Phase 1: Go Source Code & Test Harness Refactoring' (Protocol in workflow.md)

## Phase 2: Dependency & Build System Purge
- [x] Task: Clean Go module dependencies (5eb0772)
    - [x] Subtask: Remove `cloud.google.com/go/spanner` from `go.mod`
    - [x] Subtask: Run `go mod tidy` and verify clean build
- [x] Task: Purge Bazel build definitions and emulator test environment flags (7a5e34f)
    - [x] Subtask: Remove `@com_google_cloud_go_spanner` from `deps.bzl`, `MODULE.bazel`, and service `BUILD.bazel` files
    - [x] Subtask: Remove `SPANNER_EMULATOR_ADDRESS` from `.bazelrc`
- [x] Task: Conductor - User Manual Verification 'Phase 2: Dependency & Build System Purge' (Protocol in workflow.md)

## Phase 3: Infrastructure, Manifests & Documentation Cleanup
- [x] Task: Purge Spanner from Terraform modules and sample configurations cfbf4d9
    - [x] Subtask: Delete `terraform/modules/internal/spanner`
    - [x] Subtask: Remove Spanner resources, variables, and IAM bindings from `terraform/modules/buyer`, `terraform/modules/seller`, and `terraform/examples/sample`
    - [x] Subtask: Update deployment config templates (`bap-apis-config.yaml`, `seller-adapter-config.yaml`, etc.) to remove Spanner instance/database fields
- [x] Task: Clean documentation, comments, and perform final repository audit 0909311
    - [x] Subtask: Remove obsolete Spanner references and instructions across documentation and code comments
    - [x] Subtask: Verify `grep -ri "spanner" .` produces zero active code or build references
    - [x] Subtask: Execute full test suite `go test -v -race ./...`
- [ ] Task: Conductor - User Manual Verification 'Phase 3: Infrastructure, Manifests & Documentation Cleanup' (Protocol in workflow.md)
