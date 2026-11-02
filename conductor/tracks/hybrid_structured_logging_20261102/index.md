# Track: Hybrid Structured Logging & Rotation (`shared/logging`)

- **Track ID:** `hybrid_structured_logging_20261102`
- **Type:** Feature
- **Status:** In Progress
- **Specification:** [spec.md](./spec.md)
- **Implementation Plan:** [plan.md](./plan.md)
- **Metadata:** [metadata.json](./metadata.json)

## Goals
1. Establish a zero-dependency structured logging package (`shared/logging`) based on Go standard library `log/slog`.
2. Implement thread-safe size-based file rotation and backup pruning (`LogRotator`).
3. Multiplex logs into human-friendly text for console (`os.Stderr`) and machine-readable JSONL for disk files (`MultiHandler`).
4. Preserve exact emitter call-site file and line numbers using runtime frame inspection.
5. Provide a drop-in compatibility facade for legacy `glog` call sites to enable zero-friction migration.
6. Wire structured logging options (`--log-dir`, `--log-level`, `--log-max-size`, `--log-max-backups`) into the Unified Node CLI (`cmd/ondc-node`).
