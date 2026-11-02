# Specification: Unified Self-Sovereign Node & AutoCert Daemon (`cmd/ondc-node`)

## 1. Overview
In a self-sovereign deployment mode, an ONDC participant node can run either:
1. Behind a firewall/NAT via a Cloudflare Tunnel (with TLS terminated at Cloudflare edge, proxied over standard HTTP locally).
2. On a standalone Virtual Machine (VM) directly exposed to the public internet, requiring automatic TLS certificate issuance and renewal via Let's Encrypt / ACME (`golang.org/x/crypto/acme/autocert`) on port 443 with port 80 HTTP-01 challenge redirect.

This track unifies the entire ONDC node stack into a single, cohesive executable (`cmd/ondc-node`) that orchestrates:
- Onboarding & Site Verification (`/on_subscribe`, `/ondc-site-verification.html`)
- Buyer Platform (`bap-api` callback ingress and `buyer-app-service`)
- Seller Platform (`bpp-api` ingress and `callback-action-service`)
- Embedded SQLite P2P replication bridge daemon (`shared/p2p.BridgeDaemon`)
- Dual-mode network listener: Cloudflare Tunnel (plain HTTP) vs. Standalone VM (automatic HTTPS via `autocert`).

## 2. Functional Requirements

### 2.1 Unified Server Architecture (`cmd/ondc-node`)
- Single executable target `cmd/ondc-node` (and Bazel target `//cmd/ondc-node`).
- Handcrafted CLI flag parser & environment variable resolution (avoiding `spf13/cobra` or `pflag` per project rules):
  - `--mode` / `ONDC_MODE`: `tunnel` (default), `vm`, or `local`.
  - `--http-port` / `ONDC_HTTP_PORT`: Port for plain HTTP (default `8080`, or `80` in VM mode for ACME challenge/redirect).
  - `--https-port` / `ONDC_HTTPS_PORT`: Port for HTTPS (default `443` in VM mode).
  - `--domain` / `ONDC_DOMAIN`: FQDN of the node (e.g. `node.example.com` or `localhost`).
  - `--cert-dir` / `ONDC_CERT_DIR`: Directory cache for autocert keys and TLS certificates (default `./data/certs`).
  - `--db-path` / `ONDC_DB_PATH`: SQLite database file path (default `./data/ondc.db`).
  - `--crm-path` / `ONDC_CRM_PATH`: `sqlite-p2p` crm database path (default `./data/sqlite_p2p_crm.db`).
  - `--node-id` / `ONDC_NODE_ID`: Unique node ID (default hostname or auto-generated).
  - `--sync-interval` / `ONDC_SYNC_INTERVAL`: P2P replication interval (default `2s`).
  - `--enable-buyer`, `--enable-seller`, `--enable-p2p`: Toggleable feature flags (all default `true`).

### 2.2 Unified Multi-Entrypoint Routing Multiplexer
- Route prefixes mounted onto a single HTTP handler:
  - **Onboarding & Verification:**
    - `GET /ondc-site-verification.html`
    - `POST /on_subscribe`
    - `GET /.well-known/*`
  - **Buyer Platform (BAP):**
    - `POST /bap/*`
    - `POST /client/*`
  - **Seller Platform (BPP):**
    - `POST /bpp/*`
  - **P2P Replication & Node Observability:**
    - `GET /p2p/status` & `POST /p2p/sync`
    - `GET /healthz` & `GET /status` (unified health & telemetry)

### 2.3 Deployment Mode & Automatic TLS (`autocert`)
- **Mode `tunnel` or `local`:**
  - Standard HTTP server listening on `--http-port` (`:8080`).
  - Cloudflare Tunnel terminates TLS externally and proxies over HTTP.
- **Mode `vm` (Standalone VM exposed to Internet):**
  - Initialize `golang.org/x/crypto/acme/autocert.Manager` (AcceptTOS, HostWhitelist, DirCache).
  - Background HTTP listener on port 80: `m.HTTPHandler(nil)` for ACME HTTP-01 challenge and redirect to HTTPS.
  - Primary HTTPS listener on port 443 with TLSConfig utilizing `m.GetCertificate`.
  - Graceful localhost fallback when testing locally.

### 2.4 Embedded SQLite P2P Bridge Daemon Lifecycle
- Instantiates shared SQLite repository and sqlite-p2p CRM store.
- Runs `BridgeDaemon` in an embedded background goroutine.
- Coordinated graceful shutdown handling SIGINT/SIGTERM.

## 3. Non-Functional Requirements
- Strictly standard library `flag` and handcrafted resolution (no `spf13/cobra` or `pflag`).
- Test coverage >80% for server configuration, multiplexer routing, and listener setup.
- Clean shutdown within 10s timeout without resource leaks or SQLite lock errors.

## 4. Acceptance Criteria
1. `go build ./cmd/ondc-node` compiles without errors; Bazel target `//cmd/ondc-node` builds hermetically.
2. Running with `--mode=tunnel` serves all routes (`/on_subscribe`, `/bap/*`, `/bpp/*`, `/p2p/*`, `/healthz`) on the configured HTTP port.
3. Running with `--mode=vm --domain=localhost` cleanly handles local testing fallback without crashing.
4. AutoCert manager correctly initializes with the specified cache directory and domain whitelist.
5. Bidirectional SQLite P2P replication runs automatically in the background while HTTP endpoints receive requests.
6. Clean SIGINT/SIGTERM handling shuts down both the HTTP server(s) and the background P2P bridge daemon without errors.

## 5. Out of Scope
- External Kubernetes operator / Helm charts.
- Modification of ONDC protocol schemas or cryptographic signatures.
