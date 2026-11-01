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
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"cloud.google.com/go/pubsub"
	"github.com/benbjohnson/clock"
	"github.com/google/uuid"

	"ondc/shared/clients/keyclienttest"
	"ondc/shared/clients/transactionclient"
	"ondc/shared/config"
	"ondc/shared/pubsubtest"
	"ondc/shared/storage"
)

func TestCallbackActionService_InitWithSQLite(t *testing.T) {
	ctx := context.Background()
	hash := uuid.New().String()[:8]
	tmpDir := t.TempDir()
	sqliteDBFile := filepath.Join(tmpDir, fmt.Sprintf("cb_test_%s.db", hash))
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
					Filter: "attributes.action=on_search",
				},
			},
		},
	}
	_, pubsubOpt := pubsubtest.InitServer(t, projectID, psSetups)
	conf := config.CallbackActionConfig{
		ProjectID:      projectID,
		TopicID:        topicID,
		SubscriptionID: []string{subID},
		InstanceID:     "local-instance",
		DatabaseID:     databaseID,
	}

	pubsubClient, err := pubsub.NewClient(ctx, projectID, pubsubOpt)
	if err != nil {
		t.Fatalf("pubsub client failed: %v", err)
	}

	realClock := clock.New()
	keyClient := keyclienttest.NewStub(t)
	httpClient := &http.Client{}

	txnClient, err := transactionclient.New(ctx, projectID, "local-instance", databaseID)
	if err != nil {
		t.Fatalf("transactionclient.New failed: %v", err)
	}

	srv, err := initServer(ctx, httpClient, pubsubClient, keyClient, txnClient, conf, realClock)
	if err != nil {
		t.Fatalf("initServer failed: %v", err)
	}
	if srv == nil {
		t.Fatal("expected srv to not be nil")
	}

	// Verify persistence can be written and read using transaction client
	err = txnClient.StoreTransaction(ctx, transactionclient.TransactionData{
		ID:        "txn-cb-1",
		Type:      "CALLBACK-ACTION",
		API:       "on_search",
		MessageID: "msg-cb-1",
		Payload:   map[string]any{"action": "on_search"},
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

	rec, err := repo.GetTransaction(ctx, "txn-cb-1", "msg-cb-1")
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}
	if rec == nil || rec.TransactionID != "txn-cb-1" {
		t.Fatalf("expected transaction record to be retrieved, got %+v", rec)
	}

	// Verify DB file was created on disk
	if _, err := os.Stat(sqliteDBFile); os.IsNotExist(err) {
		t.Fatalf("expected SQLite file %s to exist", sqliteDBFile)
	}
}
