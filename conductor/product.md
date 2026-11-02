# Initial Concept
An enterprise-grade Go implementation of ONDC (Open Network for Digital Commerce) Open Commerce protocol adapters and microservices, enabling network participants (Buyer and Seller apps) to integrate seamlessly with the ONDC retail network.

# Product Overview
ONDC Open Commerce provides reference and production-ready middleware services designed to decouple buyers, sellers, and platforms within the digital commerce ecosystem. It standardizes communication according to ONDC Protocol v1.2.0 (Retail Domain), handling protocol compliance, cryptographic signatures, schema validation, asynchronous transaction synchronization, and secure key lifecycle management.

# Target Audience & Personas
- **Buyer Network Participants (BAP / Buyer App Platforms):** Organizations seeking to route buyer searches, orders, and fulfillment tracking to ONDC sellers.
- **Seller Network Participants (BPP / Seller App Platforms):** Retailers and marketplaces seeking to expose inventory and catalog search, order management, and fulfillment to ONDC buyers.
- **DevOps & Platform Engineers:** Teams deploying and maintaining scalable ONDC gateway adapters using Go, Bazel, containerized microservices, and peer-to-peer persistence.

# Core Capabilities & Key Features
1. **Buyer Platform (`buyer-platform`):**
   - `bap-api`: Ingress APIs receiving ONDC callback responses (`on_search`, `on_select`, `on_init`, `on_confirm`, etc.).
   - `buyer-app-service`: Client-facing services for initiating client transactions (`search`, `select`, `init`, `confirm`, `track`, `status`, `cancel`).
   - `bap-adapter-service` & `request-action-service`: Authentication, signing/verification, and asynchronous response mapping.
2. **Seller Platform (`seller-platform`):**
   - `bpp-api`: Ingress endpoint receiving requests from ONDC Gateways and BAPs.
   - `seller-adapter-service`: Validating request schemas, verifying headers, and interfacing with seller backend systems.
   - `callback-action-service`: Producing authenticated asynchronous callbacks to BAPs.
3. **Onboarding & Verification (`onboarding`):**
   - Implements `/on_subscribe` and challenge verification (`/ondc-site-verification.html`) for staging and production registry onboarding.
4. **Key Lifecycle Management (`key-rotation`):**
   - Ed25519 signing key and X25519 encryption key generation, storage, and rotation.
5. **Data & Audit Persistence (sqlite-p2p):**
   - Decentralized peer-to-peer transaction logging, audit trail, and state replication using [`sqlite-p2p`](https://github.com/innomon/sqlite-p2p), replacing external Cloud Spanner dependencies.
6. **Unified Sovereign Node (`cmd/ondc-node`):**
   - Single-executable server running onboarding, buyer, seller, and sqlite-p2p replicator together with dual-mode deployment (Cloudflare Tunnel vs Standalone VM with automated Let's Encrypt TLS).

# Non-Functional Requirements
- **Security:** Strict header signature verification (Ed25519) and payload validation against ONDC Retail OpenAPI specifications.
- **Performance & Reliability:** Microservice architecture with traffic shaping and local-first P2P replicated state.
- **Reproducibility:** Hermetic Bazel builds and containerized microservice deployments.
