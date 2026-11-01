# Specification: P2P Peer Discovery and Sync Protocol for sqlite-p2p Nodes

## 1. Overview
This track implements a decentralized Peer-to-Peer (P2P) discovery and bidirectional replication engine for `sqlite-p2p` nodes running within the ONDC network adapter architecture. The engine enables distributed ONDC participant nodes (e.g. buyer apps, seller adapters, and regional gateways) to discover adjacent peers, maintain synchronized transaction logs and participant registration records, and resolve concurrent updates without relying on centralized cloud databases like Google Cloud Spanner.

## 2. Functional Requirements
1. **Peer Discovery Service:**
   - **Hybrid Discovery:** Support local network peer discovery via multicast DNS (mDNS) and wide-area discovery via configured static/bootstrap seed peers.
   - **Peer Registry:** Maintain dynamic peer states (active, degraded, disconnected), latency metrics, and handshake statuses.
   - **Periodic Heartbeat & Liveness Daemon:** Continuously check peer health and evict unresponsive peers after configurable timeouts.
2. **Replication & Synchronization Engine:**
   - **Incremental Change-Log Replication:** Track table mutations on `transactions` and `registrations` with logical timestamps (Lamport / Vector clocks).
   - **Bidirectional Sync Protocol:** Implement push/pull delta sync protocol streaming change batches between connected peers.
   - **Conflict Resolution:** Deterministic Last-Write-Wins (LWW) resolution parameterized by Lamport timestamp + deterministic node ID tie-breaking.
3. **Transport Security & Authentication:**
   - **Cryptographic Authentication:** Authenticate peers using Ed25519 signing keys matching participant credentials in ONDC keystores.
   - **Secure Channel:** Mutual authentication and encrypted communication across peers for delta transfer.
4. **Operations & Observability:**
   - **Health & Status HTTP Endpoint:** Expose `/p2p/status` reporting active peers, sync state, latest logical clock, and replication lag.

## 3. Non-Functional Requirements
- **Hermetic Unit Testing:** All discovery and sync protocols must be testable locally in-memory or over loopback with zero external cloud dependencies.
- **Strict Go Guidelines:** Handcrafted CLI/command routing without `spf13` (Cobra/Pflag).
- **Test Coverage:** Maintain >80% test coverage across all newly introduced sync and discovery packages.

## 4. Out of Scope
- Global distributed consensus (e.g. Raft/Paxos leader election) — nodes operate as peer-to-peer eventual consistency replicas.
- Full database blob binary file copying during routine sync (only incremental mutation deltas are replicated).
