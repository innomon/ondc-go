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

	"cloud.google.com/go/pubsub"
	"github.com/benbjohnson/clock"
	"github.com/google/uuid"

	"ondc/shared/clients/registryclienttest"
	"ondc/shared/clients/transactionclient"
	"ondc/shared/config"
	"ondc/shared/pubsubtest"
	"ondc/shared/storage"
)

func TestBPPAPI_InitWithSQLite(t *testing.T) {
	ctx := context.Background()
	hash := uuid.New().String()[:8]
	tmpDir := t.TempDir()
	sqliteDBFile := filepath.Join(tmpDir, fmt.Sprintf("bpp_test_%s.db", hash))
	databaseID := fmt.Sprintf("sqlite:%s", sqliteDBFile)

	projectID := fmt.Sprintf("test-project-%s", hash)
	topicID := fmt.Sprintf("bpp-topic-%s", hash)

	psSetups := []pubsubtest.PubsubSetup{
		{TopicID: topicID},
	}
	_, opt := pubsubtest.InitServer(t, projectID, psSetups)
	conf := config.BPPAPIConfig{
		ProjectID:  projectID,
		TopicID:    topicID,
		DatabaseID: databaseID,
	}

	pubsubClient, err := pubsub.NewClient(ctx, projectID, opt)
	if err != nil {
		t.Fatalf("pubsub client failed: %v", err)
	}

	txnClient, err := transactionclient.New(ctx, projectID, "local-instance", databaseID)
	if err != nil {
		t.Fatalf("transactionclient.New failed: %v", err)
	}

	stubRegClient := registryclienttest.NewStub()
	clk := clock.New()

	srv, err := initServer(ctx, conf, stubRegClient, pubsubClient, txnClient, clk)
	if err != nil {
		t.Fatalf("initServer failed: %v", err)
	}
	if srv == nil {
		t.Fatal("expected srv to not be nil")
	}

	// Verify persistence via SQLite storage repo
	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DBPath: sqliteDBFile,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite repository: %v", err)
	}
	defer repo.Close()

	if _, err := os.Stat(sqliteDBFile); os.IsNotExist(err) {
		t.Fatalf("expected SQLite file %s to exist", sqliteDBFile)
	}
}
