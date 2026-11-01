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

package transactionclient_test

import (
	"context"
	"testing"
	"time"

	"ondc/shared/clients/transactionclient"
	"ondc/shared/storage"
)

func TestSQLiteTransactionClient_StoreTransaction(t *testing.T) {
	ctx := context.Background()

	client, err := transactionclient.New(ctx, "local-proj", "local-inst", "sqlite::memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite transactionclient: %v", err)
	}

	txData := transactionclient.TransactionData{
		ID:              "txn-abc-123",
		Type:            "REQUEST-ACTION",
		API:             "search",
		MessageID:       "msg-xyz-789",
		Payload:         map[string]any{"action": "search"},
		ProviderID:      "prv-retail-99",
		MessageStatus:   "ACK",
		ReqReceivedTime: time.Now().UTC(),
	}

	if err := client.StoreTransaction(ctx, txData); err != nil {
		t.Fatalf("StoreTransaction failed: %v", err)
	}

	// Invalid type
	invalidTx := txData
	invalidTx.Type = "INVALID-TYPE"
	if err := client.StoreTransaction(ctx, invalidTx); err == nil {
		t.Fatal("expected error on invalid transaction type, got nil")
	}

	// Invalid API
	invalidTx = txData
	invalidTx.API = "invalid_api"
	if err := client.StoreTransaction(ctx, invalidTx); err == nil {
		t.Fatal("expected error on invalid transaction API, got nil")
	}
}

func TestNewSQLiteClient_Direct(t *testing.T) {
	ctx := context.Background()
	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	client := transactionclient.NewSQLiteClient(repo)
	txData := transactionclient.TransactionData{
		ID:              "txn-direct-1",
		Type:            "CALLBACK-ACTION",
		API:             "on_search",
		MessageID:       "msg-direct-1",
		Payload:         `{"message":{"catalog":{}}}`,
		ProviderID:      "prv-1",
		ReqReceivedTime: time.Now().UTC(),
	}

	if err := client.StoreTransaction(ctx, txData); err != nil {
		t.Fatalf("StoreTransaction failed: %v", err)
	}

	// Verify in repo
	got, err := repo.GetTransaction(ctx, txData.ID, txData.MessageID)
	if err != nil {
		t.Fatalf("failed to get transaction from repo: %v", err)
	}
	if got.TransactionID != txData.ID {
		t.Errorf("got transaction ID %q, want %q", got.TransactionID, txData.ID)
	}
}
