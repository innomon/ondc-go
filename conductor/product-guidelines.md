# Product Guidelines & Engineering Standards

## 1. Architectural & Design Principles
- **Decoupled Adapter Middleware:** The adapters (`bap-adapter-service`, `seller-adapter-service`) isolate internal application systems from ONDC protocol nuances. Core domain logic must remain decoupled from protocol transport details.
- **Protocol Fidelity:** Strict compliance with ONDC Protocol v1.2.0 and OpenAPI 1.0.31 specifications. Do not alter or omit mandatory ONDC context fields (`domain`, `action`, `bap_id`, `bpp_id`, `transaction_id`, `message_id`, `timestamp`).
- **Asynchronous Reliability:** Ensure asynchronous request-response pairings (`search` -> `on_search`, etc.) maintain exact correlation via `transaction_id` and `message_id`. Acknowledge incoming requests synchronously with standardized `ACK`/`NACK` before dispatching async processing.

## 2. Storage & Persistence Architecture (`sqlite-p2p`)
- **P2P Embedded Persistence:** Replace external managed cloud databases (Cloud Spanner) with [`sqlite-p2p`](https://github.com/innomon/sqlite-p2p) for local-first, peer-to-peer replicated transaction logging, state synchronization, and audit records.
- **Decentralized Synchronization:** Transaction records and registry state are maintained locally in SQLite and synchronized asynchronously across peers using sqlite-p2p protocols.
- **Zero Cloud Lock-In:** Facilitate sovereign and edge-deployable node topologies without requiring proprietary cloud database infra.

## 3. Security & Cryptographic Standards
- **Header Authentication:** Every outbound request must generate an `Authorization` header containing an Ed25519 signature computed over the blake2b digest of the payload. Every inbound request must be cryptographically verified against the sender's public key from the ONDC registry.
- **Secret Hygiene:** Cryptographic signing and encryption keys must never be hardcoded or committed to git; manage them securely through environment configs or secure key vaults.
- **Payload Validation:** Validate all request payloads using schema validators (`validator/v10` and OpenAPI spec rules) before forwarding to internal services.

## 4. Code Quality & Idiomatic Go
- **Hermetic & Canonical:** Build rules should remain managed via Bazel (`BUILD.bazel`) and standard Go modules (`go.mod`).
- **Error Handling:** Return explicit errors with meaningful context. Avoid naked panics; handle failures gracefully and log structured details.
- **Test-Driven Rigor:** Maintain table-driven tests and payload fixtures (`testdata/*.json`) for every endpoint and handler, verifying both positive (`ack_response.json`) and negative (`nack_response.json`) execution paths.

## 5. Operational & Observability Standards
- **Transaction Auditing:** Persist audit events, peer transaction states, and request metadata via `sqlite-p2p` without blocking critical request paths.
- **Traffic Shaping:** Rate-limit and buffer outbound callbacks to prevent cascading overload on partner systems.
