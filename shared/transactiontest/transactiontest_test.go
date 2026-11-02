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

package transactiontest_test

import (
	"context"
	"testing"
	"time"

	"ondc/shared/clients/transactionclient"
	"ondc/shared/transactiontest"
)

func TestNewDatabase_ProvidesSQLitePersistence(t *testing.T) {
	ctx := context.Background()
	projectID := "test-proj-sqlite"
	instanceID := "test-inst-sqlite"
	databaseID := "test-db-sqlite"

	opts := transactiontest.NewDatabase(ctx, t, projectID, instanceID, databaseID)
	client, err := transactionclient.New(ctx, projectID, instanceID, databaseID, opts...)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	txData := transactionclient.TransactionData{
		ID:              "txn-test-1",
		Type:            "REQUEST-ACTION",
		API:             "search",
		MessageID:       "msg-test-1",
		Payload:         map[string]any{"catalog": "test"},
		ProviderID:      "prv-1",
		MessageStatus:   "ACK",
		ReqReceivedTime: time.Now().UTC(),
	}

	if err := client.StoreTransaction(ctx, txData); err != nil {
		t.Fatalf("StoreTransaction failed: %v", err)
	}
}
