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

// Package transactiontest provides helper functions for creating test transaction databases.
package transactiontest

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"ondc/shared/clients/transactionclient"
)

// NewDatabase creates a transient SQLite database for storing transaction data hermetically during tests.
func NewDatabase(ctx context.Context, t *testing.T, projectID, instanceID, databaseID string) []option.ClientOption {
	t.Helper()

	dbKey := strings.Split(databaseID, ":")[0]
	// Register this databaseID in txnClientReg if not already registered to ensure isolation
	transactionclient.RegisterTransactionClient(dbKey, func(ctx context.Context, projectID, instanceID, databaseID string) (transactionclient.Client, error) {
		return transactionclient.New(ctx, projectID, instanceID, "sqlite::memory:")
	})

	return nil
}
