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

package storage_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"ondc/shared/storage"
)

func TestSQLiteRepository_LifecycleAndCRUD(t *testing.T) {
	ctx := context.Background()

	// In-memory repo
	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DBPath: ":memory:",
	})
	if err != nil {
		t.Fatalf("NewSQLiteRepository failed: %v", err)
	}
	defer repo.Close()

	now := time.Now().UTC().Truncate(time.Second)
	tx := storage.TransactionRecord{
		TransactionID:   "txn-100",
		TransactionType: "REQUEST-ACTION",
		TransactionAPI:  "search",
		MessageID:       "msg-200",
		RequestID:       "req-300",
		Payload:         `{"context":{"domain":"retail"}}`,
		ProviderID:      "prv-500",
		MessageStatus:   "ACK",
		ReqReceivedTime: now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// 1. Save Transaction
	if err := repo.SaveTransaction(ctx, tx); err != nil {
		t.Fatalf("SaveTransaction failed: %v", err)
	}

	// 2. Get Transaction
	gotTx, err := repo.GetTransaction(ctx, tx.TransactionID, tx.MessageID)
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}
	if gotTx.TransactionID != tx.TransactionID || gotTx.MessageID != tx.MessageID {
		t.Errorf("GetTransaction mismatch: got %+v, want %+v", gotTx, tx)
	}

	// 3. Update Transaction (Upsert)
	tx.MessageStatus = "NACK"
	tx.ErrorCode = "30000"
	tx.ErrorMessage = "invalid request"
	if err := repo.SaveTransaction(ctx, tx); err != nil {
		t.Fatalf("SaveTransaction (upsert) failed: %v", err)
	}
	updatedTx, err := repo.GetTransaction(ctx, tx.TransactionID, tx.MessageID)
	if err != nil {
		t.Fatalf("GetTransaction after upsert failed: %v", err)
	}
	if updatedTx.MessageStatus != "NACK" || updatedTx.ErrorCode != "30000" {
		t.Errorf("expected updated message status NACK, got %q", updatedTx.MessageStatus)
	}

	// 4. List Transactions
	list, err := repo.ListTransactions(ctx, storage.TransactionFilter{
		TransactionID: tx.TransactionID,
	})
	if err != nil {
		t.Fatalf("ListTransactions failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(list))
	}

	// Test filters in ListTransactions
	listByMsg, err := repo.ListTransactions(ctx, storage.TransactionFilter{
		MessageID: tx.MessageID,
	})
	if err != nil || len(listByMsg) != 1 {
		t.Fatalf("expected 1 result by MessageID, got %d (err: %v)", len(listByMsg), err)
	}

	listByPrv, err := repo.ListTransactions(ctx, storage.TransactionFilter{
		ProviderID: tx.ProviderID,
	})
	if err != nil || len(listByPrv) != 1 {
		t.Fatalf("expected 1 result by ProviderID, got %d (err: %v)", len(listByPrv), err)
	}

	// 5. Save & Get Registration
	reg := storage.RegistrationRecord{
		RegistrationID:   "reg-sub-1",
		ONDCEnvironment:  "staging",
		ONDCRegistryURL:  "https://staging.registry.ondc.org",
		EntityName:       "Seller App",
		SubscriberID:     "bpp.seller.com",
		SubscriberURL:    "https://bpp.seller.com/ondc",
		SigningPublicKey: "pubkey123",
		EncrPublicKey:    "encrkey123",
		CreationTime:     now,
		LastModifiedTime: now,
	}
	if err := repo.SaveRegistration(ctx, reg); err != nil {
		t.Fatalf("SaveRegistration failed: %v", err)
	}

	gotReg, err := repo.GetRegistration(ctx, reg.RegistrationID)
	if err != nil {
		t.Fatalf("GetRegistration failed: %v", err)
	}
	if gotReg.SubscriberID != reg.SubscriberID {
		t.Errorf("expected subscriber ID %q, got %q", reg.SubscriberID, gotReg.SubscriberID)
	}

	gotBySub, err := repo.GetRegistrationBySubscriberID(ctx, reg.SubscriberID)
	if err != nil {
		t.Fatalf("GetRegistrationBySubscriberID failed: %v", err)
	}
	if gotBySub.RegistrationID != reg.RegistrationID {
		t.Errorf("expected reg ID %q, got %q", reg.RegistrationID, gotBySub.RegistrationID)
	}
}

func TestSQLiteRepository_FileCreationAndWAL(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "sub", "ondc.db")

	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DBPath:    dbPath,
		EnableWAL: true,
	})
	if err != nil {
		t.Fatalf("failed to create repo with file path: %v", err)
	}
	defer repo.Close()

	// Verify we can insert and query
	tx := storage.TransactionRecord{
		TransactionID:   "tx-file-1",
		TransactionType: "REQ",
		TransactionAPI:  "search",
		MessageID:       "msg-file-1",
		RequestID:       "req-file-1",
		Payload:         `{"test":true}`,
		ProviderID:      "prv-1",
	}
	if err := repo.SaveTransaction(ctx, tx); err != nil {
		t.Fatalf("failed to save transaction in file db: %v", err)
	}

	got, err := repo.GetTransaction(ctx, tx.TransactionID, tx.MessageID)
	if err != nil {
		t.Fatalf("failed to get transaction from file db: %v", err)
	}
	if got.TransactionID != tx.TransactionID {
		t.Errorf("mismatch: got %q, want %q", got.TransactionID, tx.TransactionID)
	}
}

func TestSQLiteRepository_Errors(t *testing.T) {
	ctx := context.Background()

	// Invalid options
	_, err := storage.NewSQLiteRepository(storage.SQLiteOptions{})
	if err == nil {
		t.Fatal("expected error on empty DB and DBPath, got nil")
	}

	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	// Test with existing *sql.DB
	rawDB, err := storage.OpenSQLiteDB(":memory:")
	if err == nil {
		defer rawDB.Close()
		repoWithDB, err := storage.NewSQLiteRepository(storage.SQLiteOptions{DB: rawDB})
		if err != nil {
			t.Fatalf("failed with existing DB: %v", err)
		}
		_ = repoWithDB.Close()
	}

	// Invalid input for SaveTransaction
	err = repo.SaveTransaction(ctx, storage.TransactionRecord{})
	if err != storage.ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}

	// Invalid input for SaveRegistration
	err = repo.SaveRegistration(ctx, storage.RegistrationRecord{})
	if err != storage.ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}

	// Invalid input for Get operations
	if _, err := repo.GetTransaction(ctx, "", ""); err != storage.ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
	if _, err := repo.GetRegistration(ctx, ""); err != storage.ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
	if _, err := repo.GetRegistrationBySubscriberID(ctx, ""); err != storage.ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}

	// Not found
	_, err = repo.GetTransaction(ctx, "none", "none")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	_, err = repo.GetRegistration(ctx, "none")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	_, err = repo.GetRegistrationBySubscriberID(ctx, "none")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

type mockTracker struct {
	mutations []string
}

func (m *mockTracker) RecordMutation(ctx context.Context, table, entityID, op, payload string) error {
	m.mutations = append(m.mutations, table+":"+entityID+":"+op)
	return nil
}

func TestSQLiteRepository_ChangeTrackerHook(t *testing.T) {
	ctx := context.Background()
	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DBPath: ":memory:",
	})
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	tracker := &mockTracker{}
	repo.SetChangeTracker(tracker)

	tx := storage.TransactionRecord{
		TransactionID:   "tx-mock",
		TransactionType: "BAP",
		TransactionAPI:  "search",
		MessageID:       "msg-mock",
		RequestID:       "req-mock",
		Payload:         `{"test":true}`,
		ProviderID:      "prv-1",
	}
	if err := repo.SaveTransaction(ctx, tx); err != nil {
		t.Fatalf("SaveTransaction failed: %v", err)
	}

	reg := storage.RegistrationRecord{
		RegistrationID:  "reg-mock",
		ONDCEnvironment: "STAGING",
		ONDCRegistryURL: "https://reg.ondc.org",
		EntityName:      "Test",
		SubscriberID:    "sub-mock",
		SubscriberURL:   "https://sub.mock/bap",
	}
	if err := repo.SaveRegistration(ctx, reg); err != nil {
		t.Fatalf("SaveRegistration failed: %v", err)
	}

	if len(tracker.mutations) != 2 {
		t.Fatalf("expected 2 mutations recorded, got %d", len(tracker.mutations))
	}
	if tracker.mutations[0] != "transactions:tx-mock:BAP:msg-mock:req-mock:INSERT" {
		t.Errorf("unexpected tx mutation: %s", tracker.mutations[0])
	}
	if tracker.mutations[1] != "registrations:reg-mock:INSERT" {
		t.Errorf("unexpected reg mutation: %s", tracker.mutations[1])
	}
}

