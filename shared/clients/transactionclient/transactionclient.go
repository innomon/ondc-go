// Copyright 2023 Google LLC
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

// Package transactionclient provides a client for storing ONDC transactions in persistent storage.
package transactionclient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/option"
	"ondc/shared/storage"
)

var (
	transactionTypeMap = map[string]int{
		"REQUEST-ACTION":  1,
		"CALLBACK-ACTION": 2,
	}

	transactionAPIMap = map[string]int{
		"search":     1,
		"on_search":  2,
		"select":     3,
		"on_select":  4,
		"init":       5,
		"on_init":    6,
		"confirm":    7,
		"on_confirm": 8,
		"status":     9,
		"on_status":  10,
		"track":      11,
		"on_track":   12,
		"cancel":     13,
		"on_cancel":  14,
		"update":     15,
		"on_update":  16,
		"rating":     17,
		"on_rating":  18,
		"support":    19,
		"on_support": 20,
	}
)

// TransactionData represents data of transaction logs.
type TransactionData struct {
	ID              string
	Type            string
	API             string
	MessageID       string
	Payload         any
	ProviderID      string
	MessageStatus   string
	ErrorType       string
	ErrorCode       string
	ErrorPath       string
	ErrorMessage    string
	ReqReceivedTime time.Time
}

// Client represents a persistent transaction storage client.
type Client interface {
	StoreTransaction(ctx context.Context, transaction TransactionData) error
}

type ClientFactory func(ctx context.Context, projectID, instanceID, databaseID string) (Client, error)

var txnClientReg = make(map[string]ClientFactory)

func RegisterTransactionClient(name string, clientFact ClientFactory) {
	txnClientReg[name] = clientFact
}

// New creates a new transaction client, defaulting to pure-Go embedded SQLite storage.
func New(ctx context.Context, projectID, instanceID, databaseID string, opts ...option.ClientOption) (Client, error) {
	dbKey := strings.Split(databaseID, ":")[0]
	if cfact, ok := txnClientReg[dbKey]; ok {
		return cfact(ctx, projectID, instanceID, databaseID)
	}

	// Default to pure-Go SQLite storage
	dbPath := databaseID
	if strings.HasPrefix(dbPath, "sqlite:") {
		dbPath = strings.TrimPrefix(dbPath, "sqlite:")
	}
	if dbPath == "" || dbPath == ":memory:" || (!strings.Contains(dbPath, "/") && !strings.HasSuffix(dbPath, ".db")) {
		dbPath = ":memory:"
	}

	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DBPath:    dbPath,
		EnableWAL: true,
	})
	if err != nil {
		return nil, fmt.Errorf("init default sqlite transaction client: %w", err)
	}

	return NewSQLiteClient(repo), nil
}
