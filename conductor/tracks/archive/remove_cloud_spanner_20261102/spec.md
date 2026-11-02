# Specification: Remove All Cloud Spanner References

## 1. Overview
Purge all legacy Google Cloud Spanner references, Go SDK dependencies, test harnesses, build flags, and Terraform infrastructure modules across `ondc-go`, establishing pure-Go embedded SQLite (`storage.Repository`) as the sole transactional persistence mechanism.

## 2. Functional Requirements
1. **Go Source & Client Layer:**
   - Remove `SpannerClient` and all `cloud.google.com/go/spanner` imports from `shared/clients/transactionclient`.
   - Standardize transaction client interfaces exclusively on `storage.Repository`.
2. **Test Harnesses & Emulators:**
   - Refactor `shared/transactiontest/transactiontest.go` to eliminate Google Cloud Spanner database/instance admin client code.
   - Remove all references to `SPANNER_EMULATOR_ADDRESS`. Standardize test database provisioning on pure-Go SQLite (`:memory:` or temporary disk databases).
3. **Go Modules & Build Toolchains:**
   - Remove `cloud.google.com/go/spanner` from `go.mod` and run `go mod tidy`.
   - Remove `@com_google_cloud_go_spanner` from `deps.bzl`, `MODULE.bazel`, and all `BUILD.bazel` files.
   - Remove `test --test_env=SPANNER_EMULATOR_ADDRESS=localhost:9010` from `.bazelrc`.
4. **Terraform & Deployment Manifests:**
   - Delete `terraform/modules/internal/spanner`.
   - Refactor `terraform/modules/buyer`, `terraform/modules/seller`, and `terraform/examples/sample/main.tf` to remove Spanner modules, variables, and IAM bindings.
   - Update deployment manifest templates (`bap-apis-config.yaml`, `seller-adapter-config.yaml`, etc.) to remove Spanner database and instance identifiers.
5. **Documentation & Config Cleanup:**
   - Remove obsolete Spanner references and instructions across documentation, comments, and configuration templates.

## 3. Non-Functional Requirements
- **Zero Cloud / Emulator Lock-In:** Build, test, and run locally without requiring Spanner emulators or GCP credentials.
- **Verification:** `go test -v -race ./...` must run green.
- **Test Coverage:** Maintain >80% test coverage across all affected client and storage packages.
- **Strict Go Guidelines:** Handcrafted CLI/routing without `spf13` (Cobra/Pflag).

## 4. Acceptance Criteria
- `grep -ri "spanner" .` yields zero active code, build, or infrastructure references.
- `go mod tidy` successfully cleans `go.mod` without `cloud.google.com/go/spanner`.
- All unit and integration tests pass cleanly with `go test -v -race ./...`.

## 5. Out of Scope
- Modifying the existing P2P discovery and sync protocols in `shared/p2p`.
- Modifying other unrelated cloud components (e.g. Cloud Pub/Sub or Cloud KMS if not touching Spanner).
