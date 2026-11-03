# Implementation Plan: Hybrid Structured Logging & Rotation (`shared/logging`)

## Phase 1: Core Engine: `LogRotator` & `MultiHandler` [checkpoint: 2690e40]
- [x] Task: Write unit tests for `LogRotator` and `MultiHandler` (`shared/logging/rotator_test.go`, `shared/logging/handler_test.go`) [fd03ac5]
    - [x] Test size-based rotation and timestamped backup naming
    - [x] Test backup pruning adhering to `maxBackups`
    - [x] Test thread safety under concurrent writes
    - [x] Test `MultiHandler` routing to multiple downstream handlers with level filtering
- [x] Task: Implement `LogRotator` and `MultiHandler` (`shared/logging/rotator.go`, `shared/logging/handler.go`) [e0b9506]
    - [x] Implement thread-safe `LogRotator` (`io.Writer` and `io.Closer`)
    - [x] Implement `MultiHandler` adhering to `slog.Handler`
- [x] Task: Conductor - User Manual Verification 'Phase 1: Core Engine: LogRotator & MultiHandler' (Protocol in workflow.md) [2690e40]

## Phase 2: Hybrid Logger APIs & Drop-in `glog` Compatibility Facade
- [x] Task: Write unit tests for Hybrid Logger, Caller PC preservation, and `glog` facade (`shared/logging/logger_test.go`, `shared/logging/compat_test.go`) [7cf6904]
    - [x] Test caller PC resolution matching emitter file and line number
    - [x] Test structured logging with context and attributes
    - [x] Test `glog` drop-in methods (`Infof`, `Errorf`, `Warningf`, `V(level)`, `Exitf`)
- [x] Task: Implement Hybrid Logger, Caller Traversal, and Compatibility Facade (`shared/logging/logger.go`, `shared/logging/compat.go`) [b8f7d48]
    - [x] Implement `Init(cfg Config)` with dual TextHandler + JSONHandler sinks
    - [x] Implement dynamic frame traversal with `runtime.Callers`
    - [x] Implement package-level `glog` compatibility methods
- [ ] Task: Conductor - User Manual Verification 'Phase 2: Hybrid Logger APIs & Drop-in glog Compatibility Facade' (Protocol in workflow.md)

## Phase 3: Integration into Unified Node (`cmd/ondc-node`), Legacy Services & E2E Testing
- [ ] Task: Write end-to-end integration test verifying unified logging and rotation (`test/logging_e2e_test.go`)
    - [ ] Test unified server running with file logging, verifying JSONL structure, attributes, and call-site source
    - [ ] Test simulated high-volume log writes triggering real file rotation
- [ ] Task: Integrate `shared/logging` into CLI flags, node server, and legacy middleware
    - [ ] Update `shared/node/config.go` with `--log-dir`, `--log-level`, `--log-max-size`, `--log-max-backups`
    - [ ] Wire `shared/logging` initialization into `cmd/ondc-node/main.go` and `cmd/ondc-p2p-bridge/main.go`
    - [ ] Migrate `shared/middleware/middleware.go` from `github.com/golang/glog` to `ondc/shared/logging`
    - [ ] Add `shared/logging/BUILD.bazel` target
    - [ ] Update `docs/DEVOPS.md` and `docs/ARCHITECTURE.md` with structured logging reference
- [ ] Task: Conductor - User Manual Verification 'Phase 3: Integration into Unified Node (cmd/ondc-node), Legacy Services & E2E Testing' (Protocol in workflow.md)
