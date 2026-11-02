# Specification: Standalone SQLite-P2P Auto-Replication Bridge Service

## 1. Overview
Implement a standalone replication daemon binary (`cmd/ondc-p2p-bridge`) that continuously and automatically synchronizes data bidirectionally between `ondc-go` (embedded SQLite repository + changelog) and `sqlite-p2p` (`crm_store` table).
All parameters must be fully configurable without hardcoded variables, supporting CLI flags and environment variables (CLI flags > ENV > defaults). Handcrafted command registry / standard library `flag` package (no spf13/Cobra).

## 2. Functional Requirements
1. **Configurable Daemon Configuration:**
   - Database paths: `ONDC_SQLITE_DB_PATH` (`--db-path`, default: `./data/ondc.db`), `ONDC_SQLITE_P2P_CRM_PATH` (`--crm-path`, default: `./data/sqlite_p2p_crm.db`).
   - Node identity: `ONDC_NODE_ID` (`--node-id`, default: hostname or generated UUID).
   - Sync parameters: `ONDC_P2P_SYNC_INTERVAL` (`--sync-interval`, default: `2s`), `ONDC_P2P_MAX_BATCH_SIZE` (`--max-batch-size`, default: `500`).
   - HTTP monitoring: `ONDC_BRIDGE_HTTP_ADDR` (`--http-addr`, default: `:8085`) exposing `/healthz`, `/status`, `/metrics`.
   - Priority rule: CLI flags > Environment variables > Defaults.
2. **Daemon Engine (`shared/p2p/daemon.go`):**
   - Encapsulates `SQLiteP2PBridge`, `SyncService`, `SQLiteRepository`, and background ticker.
   - Executes autonomous two-way sync: `ExportToSQLiteP2P` followed by `ImportFromSQLiteP2P` on every tick.
   - Exposes clean `Start(ctx context.Context) error` and `Stop() error` methods.
   - Graceful lifecycle: Responds to OS signals (SIGINT, SIGTERM) to complete in-flight sync passes cleanly.
3. **HTTP Health & Observability:**
   - `/healthz`: Returns 200 OK if both SQLite databases are accessible.
   - `/status`: Returns JSON stats including last sync time, export count, import count, current Lamport clock, and error counter.
4. **Standalone Binary Entrypoint (`cmd/ondc-p2p-bridge/main.go`):**
   - Standalone executable buildable via Go toolchain and Bazel (`//cmd/ondc-p2p-bridge`).
   - Uses standard library `flag` / custom registry; strictly **no spf13/cobra**.

## 3. Non-Functional Requirements & Guardrails
- 100% cloud-agnostic, zero Spanner dependencies.
- No third-party CLI frameworks (`spf13/cobra` or `pflag` forbidden by user rule).
- Pure-Go SQLite (`modernc.org/sqlite`) compatible with WAL journal mode.
- Unit and integration test coverage > 80%.

## 4. Acceptance Criteria
- Automated unit tests covering daemon configuration resolution, lifecycle, ticker execution, and HTTP health endpoints.
- Integration test validating daemon startup, background bidirectional replication, and graceful shutdown.
- Bazel build and `go build ./cmd/ondc-p2p-bridge` compile cleanly without errors.
