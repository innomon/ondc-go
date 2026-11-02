// Copyright 2026 ONDC Go Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ondc/shared/p2p"
	"ondc/shared/storage"

	_ "modernc.org/sqlite"
)

type simulatedNode struct {
	nodeID       string
	subscriberID string
	role         string
	db           *sql.DB
	repo         *storage.SQLiteRepository
	store        *p2p.ChangeLogStore
	clock        *p2p.LamportClock
	registry     *p2p.PeerRegistry
	syncSvc      *p2p.SyncService
	discovery    *p2p.DiscoveryService
	server       *httptest.Server
}

func createSimulatedNode(t *testing.T, nodeID, subscriberID, role string) *simulatedNode {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite for %s: %v", nodeID, err)
	}

	clock := p2p.NewLamportClock(0)
	store, err := p2p.NewChangeLogStore(db, nodeID, clock)
	if err != nil {
		t.Fatalf("failed to create changelog store for %s: %v", nodeID, err)
	}

	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DB: db,
	})
	if err != nil {
		t.Fatalf("failed to create sqlite repo for %s: %v", nodeID, err)
	}
	repo.SetChangeTracker(store)

	registry := p2p.NewPeerRegistry(5 * time.Minute)

	syncSvc := p2p.NewSyncService(p2p.SyncServiceConfig{
		NodeID:   nodeID,
		DB:       db,
		Store:    store,
		Clock:    clock,
		Registry: registry,
	})

	discovery := p2p.NewDiscoveryService(p2p.DiscoveryConfig{
		SubscriberID: subscriberID,
		Registry:     registry,
	})

	mux := http.NewServeMux()

	// 1. Discovery ping endpoint
	mux.HandleFunc("/p2p/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(p2p.PingResponse{
			ID:           nodeID,
			SubscriberID: subscriberID,
			Role:         role,
			PublicKey:    "mock-ed25519-key",
		})
	})

	// 2. Replication sync endpoints
	syncSvc.RegisterHTTPHandlers(mux)

	// 3. Status endpoint
	statusHandler := p2p.NewStatusHandler(p2p.StatusConfig{
		NodeID:       nodeID,
		SubscriberID: subscriberID,
		Role:         role,
		Clock:        clock,
		Store:        store,
		Registry:     registry,
		StartTime:    time.Now(),
	})
	mux.Handle("/p2p/status", statusHandler)

	server := httptest.NewServer(mux)

	return &simulatedNode{
		nodeID:       nodeID,
		subscriberID: subscriberID,
		role:         role,
		db:           db,
		repo:         repo,
		store:        store,
		clock:        clock,
		registry:     registry,
		syncSvc:      syncSvc,
		discovery:    discovery,
		server:       server,
	}
}

func (n *simulatedNode) close() {
	if n.server != nil {
		n.server.Close()
	}
	if n.db != nil {
		_ = n.db.Close()
	}
}

func TestMultiNode_P2PDiscoveryAndSyncE2E(t *testing.T) {
	ctx := context.Background()

	// 1. Initialize 3 interconnected nodes
	// Node 1: Buyer Platform (BAP)
	node1 := createSimulatedNode(t, "bap-node-1", "bap.mesh.ondc.org", "BAP")
	defer node1.close()

	// Node 2: Gateway Relay
	node2 := createSimulatedNode(t, "gateway-node-2", "gateway.mesh.ondc.org", "BG")
	defer node2.close()

	// Node 3: Seller Platform (BPP)
	node3 := createSimulatedNode(t, "bpp-node-3", "bpp.mesh.ondc.org", "BPP")
	defer node3.close()

	// 2. Perform Seed Discovery across all nodes
	probes1, err := node1.discovery.ProbeSeeds(ctx, []string{node2.server.URL, node3.server.URL})
	if err != nil || len(probes1) != 2 {
		t.Fatalf("expected Node 1 to discover 2 peers, got %d (err: %v)", len(probes1), err)
	}

	probes2, err := node2.discovery.ProbeSeeds(ctx, []string{node1.server.URL, node3.server.URL})
	if err != nil || len(probes2) != 2 {
		t.Fatalf("expected Node 2 to discover 2 peers, got %d (err: %v)", len(probes2), err)
	}

	probes3, err := node3.discovery.ProbeSeeds(ctx, []string{node1.server.URL, node2.server.URL})
	if err != nil || len(probes3) != 2 {
		t.Fatalf("expected Node 3 to discover 2 peers, got %d (err: %v)", len(probes3), err)
	}

	// Verify active peer lists
	if len(node1.registry.ListActive()) != 2 {
		t.Errorf("expected 2 active peers on Node 1, got %d", len(node1.registry.ListActive()))
	}
	if len(node2.registry.ListActive()) != 2 {
		t.Errorf("expected 2 active peers on Node 2, got %d", len(node2.registry.ListActive()))
	}
	if len(node3.registry.ListActive()) != 2 {
		t.Errorf("expected 2 active peers on Node 3, got %d", len(node3.registry.ListActive()))
	}

	// 3. Write data on Node 1 (Transaction and Registration)
	tx := storage.TransactionRecord{
		TransactionID:   "txn-mesh-100",
		TransactionType: "BAP",
		TransactionAPI:  "search",
		MessageID:       "msg-mesh-100",
		RequestID:       "req-mesh-100",
		Payload:         `{"item":{"descriptor":{"name":"Coffee"}}}`,
		ProviderID:      "prv-mesh-seller",
		MessageStatus:   "INIT",
		ReqReceivedTime: time.Now().UTC(),
	}
	if err := node1.repo.SaveTransaction(ctx, tx); err != nil {
		t.Fatalf("Node 1 SaveTransaction failed: %v", err)
	}

	reg := storage.RegistrationRecord{
		RegistrationID:  "reg-mesh-200",
		ONDCEnvironment: "STAGING",
		ONDCRegistryURL: "https://staging.registry.ondc.org",
		EntityName:      "Mesh Buyer Entity",
		SubscriberID:    "bap.mesh.ondc.org",
		SubscriberURL:   node1.server.URL,
	}
	if err := node1.repo.SaveRegistration(ctx, reg); err != nil {
		t.Fatalf("Node 1 SaveRegistration failed: %v", err)
	}

	// 4. Relay replication: Node 2 pulls delta from Node 1
	pullResp1, err := node2.syncSvc.PullFromPeer(ctx, node1.server.URL, 0)
	if err != nil {
		t.Fatalf("Node 2 pull from Node 1 failed: %v", err)
	}
	if len(pullResp1.Changes) != 2 {
		t.Fatalf("expected Node 2 to pull 2 changes from Node 1, got %d", len(pullResp1.Changes))
	}
	appliedCount2, err := node2.syncSvc.ApplyChangeBatch(ctx, pullResp1.Changes)
	if err != nil || appliedCount2 != 2 {
		t.Fatalf("Node 2 apply changes failed: applied %d (err: %v)", appliedCount2, err)
	}

	// Node 3 pulls delta from Node 2 (multi-hop / relay sync!)
	pullResp2, err := node3.syncSvc.PullFromPeer(ctx, node2.server.URL, 0)
	if err != nil {
		t.Fatalf("Node 3 pull from Node 2 failed: %v", err)
	}
	if len(pullResp2.Changes) != 2 {
		t.Fatalf("expected Node 3 to pull 2 changes from Node 2, got %d", len(pullResp2.Changes))
	}
	appliedCount3, err := node3.syncSvc.ApplyChangeBatch(ctx, pullResp2.Changes)
	if err != nil || appliedCount3 != 2 {
		t.Fatalf("Node 3 apply changes failed: applied %d (err: %v)", appliedCount3, err)
	}

	// 5. Verify Node 3 has received and persisted the records
	txOnNode3, err := node3.repo.GetTransaction(ctx, tx.TransactionID, tx.MessageID)
	if err != nil {
		t.Fatalf("GetTransaction on Node 3 failed: %v", err)
	}
	if txOnNode3.ProviderID != "prv-mesh-seller" || txOnNode3.MessageStatus != "INIT" {
		t.Errorf("Node 3 transaction mismatch: %+v", txOnNode3)
	}

	regOnNode3, err := node3.repo.GetRegistration(ctx, reg.RegistrationID)
	if err != nil {
		t.Fatalf("GetRegistration on Node 3 failed: %v", err)
	}
	if regOnNode3.EntityName != "Mesh Buyer Entity" {
		t.Errorf("Node 3 registration mismatch: %+v", regOnNode3)
	}

	// 6. Test Concurrent Conflicting Updates with LWW Resolution
	// Node 1 sets MessageStatus = "CANCELLED" with clock 10
	node1.clock.Witness(10)
	txNode1Update := tx
	txNode1Update.MessageStatus = "CANCELLED"
	if err := node1.repo.SaveTransaction(ctx, txNode1Update); err != nil {
		t.Fatalf("Node 1 update failed: %v", err)
	}

	// Node 3 sets MessageStatus = "CONFIRMED" with clock 20 (Higher Lamport Time -> Winner)
	node3.clock.Witness(20)
	txNode3Update := tx
	txNode3Update.MessageStatus = "CONFIRMED"
	if err := node3.repo.SaveTransaction(ctx, txNode3Update); err != nil {
		t.Fatalf("Node 3 update failed: %v", err)
	}

	// Sync Node 1 changes to Node 2
	changes1, _ := node1.store.GetChangesSince(ctx, pullResp1.LatestLamportTime, 50)
	_, _ = node2.syncSvc.ApplyChangeBatch(ctx, changes1)

	// Sync Node 3 changes to Node 2
	changes3, _ := node3.store.GetChangesSince(ctx, pullResp2.LatestLamportTime, 50)
	_, _ = node2.syncSvc.ApplyChangeBatch(ctx, changes3)

	// Sync winning state from Node 2 back to Node 1
	allChangesFrom2, _ := node2.store.GetChangesSince(ctx, 0, 100)
	_, _ = node1.syncSvc.ApplyChangeBatch(ctx, allChangesFrom2)

	// Verify all 3 nodes have converged to "CONFIRMED" (Node 3's higher Lamport clock won)
	tx1Final, _ := node1.repo.GetTransaction(ctx, tx.TransactionID, tx.MessageID)
	tx2Final, _ := node2.repo.GetTransaction(ctx, tx.TransactionID, tx.MessageID)
	tx3Final, _ := node3.repo.GetTransaction(ctx, tx.TransactionID, tx.MessageID)

	if tx1Final.MessageStatus != "CONFIRMED" {
		t.Errorf("Node 1 failed to converge, got: %s", tx1Final.MessageStatus)
	}
	if tx2Final.MessageStatus != "CONFIRMED" {
		t.Errorf("Node 2 failed to converge, got: %s", tx2Final.MessageStatus)
	}
	if tx3Final.MessageStatus != "CONFIRMED" {
		t.Errorf("Node 3 failed to converge, got: %s", tx3Final.MessageStatus)
	}

	// 7. Verify /p2p/status on all nodes
	for _, n := range []*simulatedNode{node1, node2, node3} {
		resp, err := http.Get(fmt.Sprintf("%s/p2p/status", n.server.URL))
		if err != nil {
			t.Fatalf("failed to query status for %s: %v", n.nodeID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status code %d for %s", resp.StatusCode, n.nodeID)
		}

		var status p2p.NodeStatus
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			t.Fatalf("failed to decode status for %s: %v", n.nodeID, err)
		}

		if status.NodeID != n.nodeID || status.SubscriberID != n.subscriberID {
			t.Errorf("status identity mismatch on %s: %+v", n.nodeID, status)
		}
		if len(status.ActivePeers) != 2 {
			t.Errorf("expected 2 active peers on %s, got %d", n.nodeID, len(status.ActivePeers))
		}
		if status.TotalChanges < 2 {
			t.Errorf("expected total changes >= 2 on %s, got %d", n.nodeID, status.TotalChanges)
		}
	}
}
