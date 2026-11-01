# Specification: Integrate sqlite-p2p persistence and Cloudflare Tunnel local deployment

## Overview
This track replaces Google Cloud Spanner persistence in the ONDC Go implementation with [`sqlite-p2p`](https://github.com/innomon/sqlite-p2p), enabling sovereign, embedded, peer-to-peer replicated transaction audit and registration storage. Additionally, it introduces local deployment workflows leveraging Cloudflare Tunnel (`cloudflared`) to securely expose local BAP/BPP services and verify ONDC site challenges over public HTTPS without cloud infrastructure dependencies or open router ports.

## Problem Statement & Goals
- **Decouple from Managed Cloud DB:** The original architecture strictly relies on Google Cloud Spanner and Pub/Sub for storing transaction events and participant registration. For local, sovereign, and peer-to-peer deployments, an embedded P2P-synchronized SQLite database is required.
- **Enable Local-First Ingress:** ONDC network participants require publicly reachable HTTPS URLs with valid certificates for registry lookup callbacks, `/on_subscribe` verification (`/ondc-site-verification.html`), and BAP/BPP asynchronous callbacks. Cloudflare Tunnel provides an automated, zero-trust HTTPS tunnel for local developers.

## Scope & Functional Requirements

### 1. Storage Abstraction & sqlite-p2p Integration
- Define a canonical `Storage` / `Repository` interface representing transaction logging, message correlation, and participant registry records.
- Implement the SQLite schema matching Spanner's transaction and registration tables:
  - `transactions` (transaction_id, message_id, action, bpp_id, bap_id, status, payload, created_at, updated_at).
  - `registrations` (subscriber_id, uk_id, signing_public_key, encr_public_key, valid_from, valid_until, created_at).
- Integrate `sqlite-p2p` client library to handle local database reads/writes and background P2P replication across network nodes.
- Refactor buyer and seller adapters (`buyer-platform`, `seller-platform`) to utilize the storage interface instead of direct Spanner client calls.

### 2. Cloudflare Tunnel Local Deployment Tooling
- Provide configuration templates (`cloudflared/config.yaml`) and runtime scripts (`scripts/run_local_tunnel.sh`) to start local services and bind tunnels.
- Route incoming public tunnel subdomains:
  - Ingress to Buyer Platform API (`bap-api` / callback receiver).
  - Ingress to Seller Platform API (`bpp-api` / request receiver).
  - Ingress to Onboarding Service (`/ondc-site-verification.html`).
- Document key generation and site verification workflow when operating over Cloudflare Tunnel.

### 3. Verification & Testing
- Unit tests covering SQLite schema creation, CRUD operations, transaction history queries, and error handling.
- Integration tests simulating an end-to-end asynchronous transaction (`search` -> `on_search`) with sqlite-p2p record persistence.
- Local health check and verification script validating Cloudflare Tunnel routing to local service ports.
