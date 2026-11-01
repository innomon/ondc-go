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

package p2p

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ondc/shared/storage"

	_ "modernc.org/sqlite"
)

func TestConflictResolution_LWW(t *testing.T) {
	now := time.Now().UTC()

	existing := ChangeRecord{
		ChangeID:    "chg-1",
		TableName:   "transactions",
		EntityID:    "txn-1",
		Operation:   OpInsert,
		LamportTime: 10,
		NodeID:      "node-a",
		Payload:     `{"status":"ACK"}`,
		CreatedAt:   now,
	}

	// Case 1: Incoming has higher Lamport time -> Wins
	incomingHigher := ChangeRecord{
		ChangeID:    "chg-2",
		TableName:   "transactions",
		EntityID:    "txn-1",
		Operation:   OpUpdate,
		LamportTime: 11,
		NodeID:      "node-b",
		Payload:     `{"status":"NACK"}`,
		CreatedAt:   now,
	}
	if !ShouldApply(existing, incomingHigher) {
		t.Errorf("expected incomingHigher with Lamport 11 to win over 10")
	}

	// Case 2: Incoming has lower Lamport time -> Loses
	incomingLower := ChangeRecord{
		ChangeID:    "chg-0",
		TableName:   "transactions",
		EntityID:    "txn-1",
		Operation:   OpUpdate,
		LamportTime: 9,
		NodeID:      "node-b",
		Payload:     `{"status":"PENDING"}`,
		CreatedAt:   now,
	}
	if ShouldApply(existing, incomingLower) {
		t.Errorf("expected incomingLower with Lamport 9 to lose against 10")
	}

	// Case 3: Same Lamport time, NodeID tie-breaker
	// "node-b" > "node-a" -> incoming wins
	incomingTieWin := ChangeRecord{
		ChangeID:    "chg-3",
		TableName:   "transactions",
		EntityID:    "txn-1",
		Operation:   OpUpdate,
		LamportTime: 10,
		NodeID:      "node-b",
		Payload:     `{"status":"TIE_WIN"}`,
		CreatedAt:   now,
	}
	if !ShouldApply(existing, incomingTieWin) {
		t.Errorf("expected node-b to win tie against node-a")
	}

	// "node-0" < "node-a" -> incoming loses
	incomingTieLose := ChangeRecord{
		ChangeID:    "chg-4",
		TableName:   "transactions",
		EntityID:    "txn-1",
		Operation:   OpUpdate,
		LamportTime: 10,
		NodeID:      "node-0",
		Payload:     `{"status":"TIE_LOSE"}`,
		CreatedAt:   now,
	}
	if ShouldApply(existing, incomingTieLose) {
		t.Errorf("expected node-0 to lose tie against node-a")
	}
}

func setupTestNode(t *testing.T, nodeID string) (*sql.DB, *storage.SQLiteRepository, *ChangeLogStore, *SyncService) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	clock := NewLamportClock(0)
	store, err := NewChangeLogStore(db, nodeID, clock)
	if err != nil {
		t.Fatalf("failed to create changelog store: %v", err)
	}

	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DB: db,
	})
	if err != nil {
		t.Fatalf("failed to create sqlite repo: %v", err)
	}
	repo.SetChangeTracker(store)

	syncSvc := NewSyncService(SyncServiceConfig{
		NodeID:   nodeID,
		DB:       db,
		Store:    store,
		Clock:    clock,
		Registry: NewPeerRegistry(time.Minute),
	})

	return db, repo, store, syncSvc
}

func TestSyncService_PullAndApplyChanges(t *testing.T) {
	ctx := context.Background()

	dbA, repoA, _, syncA := setupTestNode(t, "node-A")
	defer dbA.Close()

	dbB, repoB, storeB, syncB := setupTestNode(t, "node-B")
	defer dbB.Close()

	// 1. Create a transaction on Node A
	txA := storage.TransactionRecord{
		TransactionID:   "txn-sync-1",
		TransactionType: "BAP",
		TransactionAPI:  "search",
		MessageID:       "msg-sync-1",
		RequestID:       "req-sync-1",
		Payload:         `{"item":"sample-item"}`,
		ProviderID:      "prv-sync",
		ReqReceivedTime: time.Now().UTC(),
	}
	if err := repoA.SaveTransaction(ctx, txA); err != nil {
		t.Fatalf("Node A SaveTransaction failed: %v", err)
	}

	// 2. Node B requests pull sync from Node A
	pullReq := SyncRequest{
		NodeID:          "node-B",
		SinceLamportTime: 0,
		BatchSize:       50,
	}

	pullResp, err := syncA.HandlePullSync(ctx, pullReq)
	if err != nil {
		t.Fatalf("HandlePullSync failed: %v", err)
	}
	if len(pullResp.Changes) != 1 {
		t.Fatalf("expected 1 change from Node A, got %d", len(pullResp.Changes))
	}

	// 3. Node B applies the batch
	applied, err := syncB.ApplyChangeBatch(ctx, pullResp.Changes)
	if err != nil {
		t.Fatalf("ApplyChangeBatch on Node B failed: %v", err)
	}
	if applied != 1 {
		t.Fatalf("expected 1 applied change, got %d", applied)
	}

	// 4. Verify transaction exists on Node B
	gotTxB, err := repoB.GetTransaction(ctx, txA.TransactionID, txA.MessageID)
	if err != nil {
		t.Fatalf("Node B GetTransaction failed: %v", err)
	}
	if gotTxB.ProviderID != txA.ProviderID {
		t.Errorf("Node B transaction mismatch: got %v, want %v", gotTxB.ProviderID, txA.ProviderID)
	}

	// Verify changelog on Node B
	bChanges, err := storeB.GetChangesSince(ctx, 0, 10)
	if err != nil || len(bChanges) != 1 {
		t.Fatalf("expected Node B to have 1 change in changelog, got %d (err: %v)", len(bChanges), err)
	}
}

func TestSyncService_PushAndResolveConflicts(t *testing.T) {
	ctx := context.Background()

	dbA, repoA, _, syncA := setupTestNode(t, "node-A")
	defer dbA.Close()

	dbB, repoB, _, syncB := setupTestNode(t, "node-B")
	defer dbB.Close()

	// Initial registration on Node A
	reg := storage.RegistrationRecord{
		RegistrationID:  "reg-conflict-1",
		ONDCEnvironment: "STAGING",
		ONDCRegistryURL: "https://reg.ondc.org",
		EntityName:      "Initial Name",
		SubscriberID:    "sub-conflict",
		SubscriberURL:   "https://sub.conflict/bap",
	}
	if err := repoA.SaveRegistration(ctx, reg); err != nil {
		t.Fatalf("SaveRegistration failed: %v", err)
	}

	// Sync initial registration to Node B
	resp, err := syncA.HandlePullSync(ctx, SyncRequest{NodeID: "node-B", SinceLamportTime: 0})
	if err != nil {
		t.Fatalf("HandlePullSync failed: %v", err)
	}
	if _, err := syncB.ApplyChangeBatch(ctx, resp.Changes); err != nil {
		t.Fatalf("ApplyChangeBatch failed: %v", err)
	}

	// Node A makes an update at lower Lamport time
	regA := reg
	regA.EntityName = "Name From Node A"
	if err := repoA.SaveRegistration(ctx, regA); err != nil {
		t.Fatalf("repoA update failed: %v", err)
	}

	// Node B advances clock and makes an update at higher Lamport time
	syncB.clock.Witness(20) // advance Node B clock to 21
	regB := reg
	regB.EntityName = "Winner Name From Node B"
	if err := repoB.SaveRegistration(ctx, regB); err != nil {
		t.Fatalf("repoB update failed: %v", err)
	}

	// Push changes from Node B to Node A
	changesFromB, err := syncB.store.GetChangesSince(ctx, 0, 100)
	if err != nil {
		t.Fatalf("GetChangesSince Node B failed: %v", err)
	}

	pushResp, err := syncA.HandlePushSync(ctx, PushSyncRequest{
		NodeID:  "node-B",
		Changes: changesFromB,
	})
	if err != nil {
		t.Fatalf("HandlePushSync failed: %v", err)
	}
	if pushResp.AppliedCount == 0 {
		t.Fatalf("expected Node A to apply higher Lamport changes from Node B")
	}

	// Verify Node A now has Node B's winner value
	gotRegA, err := repoA.GetRegistration(ctx, reg.RegistrationID)
	if err != nil {
		t.Fatalf("Node A GetRegistration failed: %v", err)
	}
	if gotRegA.EntityName != "Winner Name From Node B" {
		t.Fatalf("expected Node A to have Winner Name, got: %s", gotRegA.EntityName)
	}
}

func TestSyncService_HTTPHandlerTransport(t *testing.T) {
	ctx := context.Background()

	dbA, repoA, _, syncA := setupTestNode(t, "node-http-A")
	defer dbA.Close()

	dbB, repoB, _, syncB := setupTestNode(t, "node-http-B")
	defer dbB.Close()

	// Setup HTTP test server for Node A
	muxA := http.NewServeMux()
	syncA.RegisterHTTPHandlers(muxA)
	serverA := httptest.NewServer(muxA)
	defer serverA.Close()

	// Seed data on Node A
	tx := storage.TransactionRecord{
		TransactionID:   "txn-http-1",
		TransactionType: "BAP",
		TransactionAPI:  "on_search",
		MessageID:       "msg-http-1",
		RequestID:       "req-http-1",
		Payload:         `{"http":true}`,
		ProviderID:      "prv-http",
		ReqReceivedTime: time.Now().UTC(),
	}
	if err := repoA.SaveTransaction(ctx, tx); err != nil {
		t.Fatalf("SaveTransaction failed: %v", err)
	}

	// Node B pulls from Node A via HTTP
	resp, err := syncB.PullFromPeer(ctx, serverA.URL, 0)
	if err != nil {
		t.Fatalf("PullFromPeer failed: %v", err)
	}
	if len(resp.Changes) != 1 {
		t.Fatalf("expected 1 change from HTTP pull, got %d", len(resp.Changes))
	}

	// Node B applies the change
	if _, err := syncB.ApplyChangeBatch(ctx, resp.Changes); err != nil {
		t.Fatalf("ApplyChangeBatch failed: %v", err)
	}

	// Verify Node B has the transaction
	gotTx, err := repoB.GetTransaction(ctx, tx.TransactionID, tx.MessageID)
	if err != nil {
		t.Fatalf("Node B GetTransaction failed: %v", err)
	}
	if gotTx.TransactionAPI != "on_search" {
		t.Errorf("expected on_search API, got %s", gotTx.TransactionAPI)
	}

	// Node B pushes back to Node A
	pushResp, err := syncB.PushToPeer(ctx, serverA.URL, resp.Changes)
	if err != nil {
		t.Fatalf("PushToPeer failed: %v", err)
	}
	_ = pushResp
}

func TestSyncService_EdgeCasesAndHTTPFailures(t *testing.T) {
	ctx := context.Background()
	db, _, _, syncSvc := setupTestNode(t, "node-edge")
	defer db.Close()

	// 1. Empty change batch
	applied, err := syncSvc.ApplyChangeBatch(ctx, nil)
	if err != nil || applied != 0 {
		t.Fatalf("expected 0 applied on empty batch, got %d, err %v", applied, err)
	}

	// 2. Unknown table in change record
	badChange := ChangeRecord{
		TableName:   "unknown_table",
		EntityID:    "xyz",
		Operation:   OpInsert,
		LamportTime: 1,
		NodeID:      "other",
		Payload:     "{}",
	}
	_, err = syncSvc.ApplyChangeBatch(ctx, []ChangeRecord{badChange})
	if err == nil {
		t.Fatalf("expected error on unknown table")
	}

	// 3. HTTP handler test with invalid methods and payloads
	mux := http.NewServeMux()
	syncSvc.RegisterHTTPHandlers(mux)

	// GET to pull endpoint -> 405
	req, _ := http.NewRequest(http.MethodGet, "/p2p/sync/pull", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 on GET /p2p/sync/pull, got %d", rec.Code)
	}

	// Invalid JSON to pull endpoint -> 400
	req, _ = http.NewRequest(http.MethodPost, "/p2p/sync/pull", httptest.NewRecorder().Body)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on invalid JSON /p2p/sync/pull, got %d", rec.Code)
	}

	// GET to push endpoint -> 405
	req, _ = http.NewRequest(http.MethodGet, "/p2p/sync/push", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 on GET /p2p/sync/push, got %d", rec.Code)
	}

	// Invalid JSON to push endpoint -> 400
	req, _ = http.NewRequest(http.MethodPost, "/p2p/sync/push", httptest.NewRecorder().Body)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on invalid JSON /p2p/sync/push, got %d", rec.Code)
	}

	// 4. Client error on 500 server response
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server failure", http.StatusInternalServerError)
	}))
	defer errServer.Close()

	_, err = syncSvc.PullFromPeer(ctx, errServer.URL, 0)
	if err == nil {
		t.Errorf("expected error when pulling from error server")
	}

	_, err = syncSvc.PushToPeer(ctx, errServer.URL, []ChangeRecord{badChange})
	if err == nil {
		t.Errorf("expected error when pushing to error server")
	}
}

