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

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/benbjohnson/clock"
	"github.com/google/uuid"
	"google.golang.org/api/option"

	"ondc/shared/clients/keyclienttest"
	"ondc/shared/clients/transactionclient"
	"ondc/shared/config"
	"ondc/shared/pubsubtest"
	"ondc/shared/storage"
)

func TestRequestActionService_InitWithSQLite(t *testing.T) {
	ctx := context.Background()
	hash := uuid.New().String()[:8]
	tmpDir := t.TempDir()
	sqliteDBFile := filepath.Join(tmpDir, fmt.Sprintf("req_test_%s.db", hash))
	databaseID := fmt.Sprintf("sqlite:%s", sqliteDBFile)

	projectID := fmt.Sprintf("test-project-%s", hash)
	topicID := fmt.Sprintf("test-topic-%s", hash)
	subID := fmt.Sprintf("test-sub-%s", hash)

	psSetups := []pubsubtest.PubsubSetup{
		{
			TopicID: topicID,
			SubSetups: []pubsubtest.SubSetup{
				{
					SubID:  subID,
					Filter: "attributes.action=search",
				},
			},
		},
	}
	_, pubsubOpt := pubsubtest.InitServer(t, projectID, psSetups)
	conf := config.RequestActionConfig{
		ProjectID:      projectID,
		SubscriptionID: []string{subID},
		InstanceID:     "local-instance",
		DatabaseID:     databaseID,
	}

	realClock := clock.New()
	keyClient := keyclienttest.NewStub(t)

	srv, err := initServer(ctx, conf, realClock, keyClient, []option.ClientOption{pubsubOpt}, nil)
	if err != nil {
		t.Fatalf("initServer failed: %v", err)
	}
	if srv == nil {
		t.Fatal("expected srv to not be nil")
	}

	// Verify persistence can be written and read using transaction client
	txnClient, err := transactionclient.New(ctx, projectID, "local-instance", databaseID)
	if err != nil {
		t.Fatalf("transactionclient.New failed: %v", err)
	}

	err = txnClient.StoreTransaction(ctx, transactionclient.TransactionData{
		ID:        "txn-req-1",
		Type:      "REQUEST-ACTION",
		API:       "search",
		MessageID: "msg-req-1",
		Payload:   map[string]any{"action": "search"},
	})
	if err != nil {
		t.Fatalf("StoreTransaction failed: %v", err)
	}

	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DBPath: sqliteDBFile,
	})
	if err != nil {
		t.Fatalf("open sqlite repo failed: %v", err)
	}
	defer repo.Close()

	rec, err := repo.GetTransaction(ctx, "txn-req-1", "msg-req-1")
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}
	if rec == nil || rec.TransactionID != "txn-req-1" {
		t.Fatalf("expected transaction record to be retrieved, got %+v", rec)
	}

	// Verify DB file was created on disk
	if _, err := os.Stat(sqliteDBFile); os.IsNotExist(err) {
		t.Fatalf("expected SQLite file %s to exist", sqliteDBFile)
	}
}
