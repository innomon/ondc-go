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
	"sync"
	"testing"
	"time"

	"ondc/shared/storage"

	_ "modernc.org/sqlite"
)

func TestLamportClock(t *testing.T) {
	clock := NewLamportClock(0)
	if clock.Current() != 0 {
		t.Fatalf("expected initial clock 0, got %d", clock.Current())
	}

	c1 := clock.Increment()
	if c1 != 1 {
		t.Fatalf("expected clock 1 after increment, got %d", c1)
	}

	c2 := clock.Witness(10)
	if c2 != 11 {
		t.Fatalf("expected clock 11 after witnessing 10, got %d", c2)
	}

	c3 := clock.Witness(5)
	if c3 != 12 {
		t.Fatalf("expected clock 12 after witnessing 5, got %d", c3)
	}

	// Concurrent increments
	var wg sync.WaitGroup
	const routines = 50
	for i := 0; i < routines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			clock.Increment()
		}()
	}
	wg.Wait()

	expected := uint64(12 + routines)
	if clock.Current() != expected {
		t.Fatalf("expected clock %d after concurrent increments, got %d", expected, clock.Current())
	}
}

func TestChangeLogStore_RecordAndRetrieve(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	clock := NewLamportClock(0)
	store, err := NewChangeLogStore(db, "node-1", clock)
	if err != nil {
		t.Fatalf("failed to create changelog store: %v", err)
	}

	// Initial latest lamport time should be 0
	latest, err := store.GetLatestLamportTime(ctx)
	if err != nil {
		t.Fatalf("failed to get latest lamport time: %v", err)
	}
	if latest != 0 {
		t.Fatalf("expected latest lamport 0, got %d", latest)
	}

	// Record changes
	change1 := ChangeRecord{
		ChangeID:    "chg-1",
		TableName:   "transactions",
		EntityID:    "txn-100:BAP:msg-1:req-1",
		Operation:   OpInsert,
		Payload:     `{"transaction_id":"txn-100"}`,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.RecordChange(ctx, change1); err != nil {
		t.Fatalf("failed to record change1: %v", err)
	}

	change2 := ChangeRecord{
		ChangeID:    "chg-2",
		TableName:   "registrations",
		EntityID:    "reg-200",
		Operation:   OpInsert,
		Payload:     `{"registration_id":"reg-200"}`,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.RecordChange(ctx, change2); err != nil {
		t.Fatalf("failed to record change2: %v", err)
	}

	changes, err := store.GetChangesSince(ctx, 0, 10)
	if err != nil {
		t.Fatalf("failed to get changes since 0: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	if changes[0].ChangeID != "chg-1" || changes[0].LamportTime != 1 {
		t.Fatalf("unexpected change[0]: %+v", changes[0])
	}
	if changes[1].ChangeID != "chg-2" || changes[1].LamportTime != 2 {
		t.Fatalf("unexpected change[1]: %+v", changes[1])
	}

	// Query since lamport 1
	changesSince1, err := store.GetChangesSince(ctx, 1, 10)
	if err != nil {
		t.Fatalf("failed to get changes since 1: %v", err)
	}
	if len(changesSince1) != 1 || changesSince1[0].ChangeID != "chg-2" {
		t.Fatalf("expected only chg-2, got %+v", changesSince1)
	}

	latest, err = store.GetLatestLamportTime(ctx)
	if err != nil {
		t.Fatalf("failed to get latest lamport time: %v", err)
	}
	if latest != 2 {
		t.Fatalf("expected latest lamport 2, got %d", latest)
	}
}

func TestSQLiteRepository_ChangeTrackerIntegration(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	clock := NewLamportClock(0)
	store, err := NewChangeLogStore(db, "node-integration", clock)
	if err != nil {
		t.Fatalf("failed to create changelog store: %v", err)
	}

	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DB: db,
	})
	if err != nil {
		t.Fatalf("failed to create sqlite repository: %v", err)
	}

	repo.SetChangeTracker(store)

	// Save transaction through repository
	txn := storage.TransactionRecord{
		TransactionID:   "txn-p2p-1",
		TransactionType: "BAP",
		TransactionAPI:  "search",
		MessageID:       "msg-p2p-1",
		RequestID:       "req-p2p-1",
		Payload:         `{"message":{"intent":{}}}`,
		ProviderID:      "prv-1",
		ReqReceivedTime: time.Now().UTC(),
	}

	if err := repo.SaveTransaction(ctx, txn); err != nil {
		t.Fatalf("SaveTransaction failed: %v", err)
	}

	// Verify changelog captured the transaction
	changes, err := store.GetChangesSince(ctx, 0, 10)
	if err != nil {
		t.Fatalf("GetChangesSince failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change record, got %d", len(changes))
	}
	if changes[0].TableName != "transactions" || changes[0].EntityID != "txn-p2p-1:BAP:msg-p2p-1:req-p2p-1" {
		t.Fatalf("unexpected change entity: %+v", changes[0])
	}
	if changes[0].NodeID != "node-integration" || changes[0].LamportTime != 1 {
		t.Fatalf("unexpected node or lamport time: %+v", changes[0])
	}

	// Save registration through repository
	reg := storage.RegistrationRecord{
		RegistrationID:  "reg-p2p-1",
		ONDCEnvironment: "STAGING",
		ONDCRegistryURL: "https://staging.registry.ondc.org",
		EntityName:      "Test Seller",
		SubscriberID:    "seller.example.com",
		SubscriberURL:   "https://seller.example.com/bpp",
	}

	if err := repo.SaveRegistration(ctx, reg); err != nil {
		t.Fatalf("SaveRegistration failed: %v", err)
	}

	// Verify changelog captured registration
	changes, err = store.GetChangesSince(ctx, 1, 10)
	if err != nil {
		t.Fatalf("GetChangesSince failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change record since lamport 1, got %d", len(changes))
	}
	if changes[0].TableName != "registrations" || changes[0].EntityID != "reg-p2p-1" {
		t.Fatalf("unexpected registration change: %+v", changes[0])
	}
	if changes[0].LamportTime != 2 {
		t.Fatalf("expected lamport time 2, got %d", changes[0].LamportTime)
	}
}

func TestChangeLogStore_ApplyRemoteChange(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	clock := NewLamportClock(5)
	store, err := NewChangeLogStore(db, "node-local", clock)
	if err != nil {
		t.Fatalf("failed to create changelog store: %v", err)
	}

	remoteChange := ChangeRecord{
		ChangeID:    "remote-chg-1",
		TableName:   "transactions",
		EntityID:    "txn-remote-1",
		Operation:   OpInsert,
		LamportTime: 15,
		NodeID:      "node-remote",
		Payload:     `{"transaction_id":"txn-remote-1"}`,
		CreatedAt:   time.Now().UTC(),
	}

	if err := store.ApplyRemoteChange(ctx, remoteChange); err != nil {
		t.Fatalf("ApplyRemoteChange failed: %v", err)
	}

	// Clock should be max(5, 15) + 1 = 16
	if clock.Current() != 16 {
		t.Fatalf("expected clock 16 after remote change, got %d", clock.Current())
	}

	// Applying duplicate change should be ignored (idempotent)
	if err := store.ApplyRemoteChange(ctx, remoteChange); err != nil {
		t.Fatalf("ApplyRemoteChange duplicate failed: %v", err)
	}

	changes, err := store.GetChangesSince(ctx, 0, 10)
	if err != nil {
		t.Fatalf("GetChangesSince failed: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
}

func TestChangeLogStore_NilDBAndResume(t *testing.T) {
	_, err := NewChangeLogStore(nil, "node-1", nil)
	if err == nil {
		t.Fatalf("expected error on nil db")
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	defer db.Close()

	clock1 := NewLamportClock(0)
	store1, err := NewChangeLogStore(db, "node-1", clock1)
	if err != nil {
		t.Fatalf("failed to create store1: %v", err)
	}

	ctx := context.Background()
	_ = store1.RecordChange(ctx, ChangeRecord{
		TableName: "transactions",
		EntityID:  "txn-1",
		Operation: OpInsert,
		Payload:   "{}",
	})

	// Reopen store on same db with clock 0, it should resume clock from DB
	clock2 := NewLamportClock(0)
	store2, err := NewChangeLogStore(db, "node-1", clock2)
	if err != nil {
		t.Fatalf("failed to create store2: %v", err)
	}

	if clock2.Current() != 1 {
		t.Fatalf("expected resumed clock 1, got %d", clock2.Current())
	}
	_ = store2
}

