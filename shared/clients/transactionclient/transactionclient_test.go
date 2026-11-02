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
)

func TestNew_DefaultToSQLiteWithoutSpanner(t *testing.T) {
	ctx := context.Background()

	// Calling New with standard project/instance/database strings should succeed
	// without any Spanner credentials or emulator, routing to SQLite storage.
	client, err := transactionclient.New(ctx, "test-proj", "test-inst", "default-tx-db")
	if err != nil {
		t.Fatalf("expected transactionclient.New to default to SQLite, got error: %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil transaction client")
	}

	txData := transactionclient.TransactionData{
		ID:              "txn-spanner-free-1",
		Type:            "REQUEST-ACTION",
		API:             "search",
		MessageID:       "msg-sf-1",
		Payload:         map[string]any{"search": "tea"},
		ProviderID:      "prv-123",
		MessageStatus:   "ACK",
		ReqReceivedTime: time.Now().UTC(),
	}

	if err := client.StoreTransaction(ctx, txData); err != nil {
		t.Fatalf("StoreTransaction failed: %v", err)
	}
}
