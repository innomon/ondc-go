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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ondc/shared/storage"
)

// ShouldApply determines whether incoming change supersedes existing change via Last-Write-Wins (LWW).
func ShouldApply(existing, incoming ChangeRecord) bool {
	if incoming.LamportTime > existing.LamportTime {
		return true
	}
	if incoming.LamportTime < existing.LamportTime {
		return false
	}
	// Tie-break deterministically using node identifier
	return incoming.NodeID > existing.NodeID
}

// SyncRequest specifies parameters for pulling mutation changes from a peer.
type SyncRequest struct {
	NodeID           string `json:"node_id"`
	SinceLamportTime uint64 `json:"since_lamport_time"`
	BatchSize        int    `json:"batch_size,omitempty"`
}

// SyncResponse returns a batch of changes and synchronization metadata.
type SyncResponse struct {
	NodeID             string         `json:"node_id"`
	LatestLamportTime  uint64         `json:"latest_lamport_time"`
	Changes            []ChangeRecord `json:"changes"`
	HasMore            bool           `json:"has_more"`
}

// PushSyncRequest packages a batch of changes pushed from a peer.
type PushSyncRequest struct {
	NodeID  string         `json:"node_id"`
	Changes []ChangeRecord `json:"changes"`
}

// PushSyncResponse provides acknowledgment and result metrics for a push sync.
type PushSyncResponse struct {
	NodeID            string `json:"node_id"`
	AppliedCount      int    `json:"applied_count"`
	LatestLamportTime uint64 `json:"latest_lamport_time"`
}

// SyncServiceConfig configures the sync service.
type SyncServiceConfig struct {
	NodeID     string
	DB         *sql.DB
	Store      *ChangeLogStore
	Clock      *LamportClock
	Registry   *PeerRegistry
	HTTPClient *http.Client
}

// SyncService coordinates push/pull delta replication and conflict resolution.
type SyncService struct {
	nodeID     string
	db         *sql.DB
	store      *ChangeLogStore
	clock      *LamportClock
	registry   *PeerRegistry
	httpClient *http.Client
}

// NewSyncService constructs a new SyncService.
func NewSyncService(cfg SyncServiceConfig) *SyncService {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &SyncService{
		nodeID:     cfg.NodeID,
		db:         cfg.DB,
		store:      cfg.Store,
		clock:      cfg.Clock,
		registry:   cfg.Registry,
		httpClient: client,
	}
}

// HandlePullSync handles an incoming pull sync request from a peer.
func (s *SyncService) HandlePullSync(ctx context.Context, req SyncRequest) (*SyncResponse, error) {
	limit := req.BatchSize
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	changes, err := s.store.GetChangesSince(ctx, req.SinceLamportTime, limit+1)
	if err != nil {
		return nil, fmt.Errorf("handle pull sync: get changes: %w", err)
	}

	hasMore := false
	if len(changes) > limit {
		hasMore = true
		changes = changes[:limit]
	}

	latest, err := s.store.GetLatestLamportTime(ctx)
	if err != nil {
		latest = s.clock.Current()
	}

	return &SyncResponse{
		NodeID:            s.nodeID,
		LatestLamportTime: latest,
		Changes:           changes,
		HasMore:           hasMore,
	}, nil
}

// HandlePushSync handles incoming pushed changes from a peer.
func (s *SyncService) HandlePushSync(ctx context.Context, req PushSyncRequest) (*PushSyncResponse, error) {
	applied, err := s.ApplyChangeBatch(ctx, req.Changes)
	if err != nil {
		return nil, fmt.Errorf("handle push sync: %w", err)
	}

	latest, err := s.store.GetLatestLamportTime(ctx)
	if err != nil {
		latest = s.clock.Current()
	}

	return &PushSyncResponse{
		NodeID:            s.nodeID,
		AppliedCount:      applied,
		LatestLamportTime: latest,
	}, nil
}

// ApplyChangeBatch resolves conflicts and applies changes atomically to the local node.
func (s *SyncService) ApplyChangeBatch(ctx context.Context, changes []ChangeRecord) (int, error) {
	if len(changes) == 0 {
		return 0, nil
	}

	appliedCount := 0

	for _, chg := range changes {
		// Look up existing latest change for the entity
		existing, err := s.getLatestEntityChange(ctx, chg.TableName, chg.EntityID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return appliedCount, fmt.Errorf("check existing change: %w", err)
		}

		if existing != nil && !ShouldApply(*existing, chg) {
			// Incoming loses conflict resolution against existing record
			// Still witness clock so logical clock does not fall behind
			s.clock.Witness(chg.LamportTime)
			continue
		}

		// Apply mutation to domain table
		if err := s.applyToDomainTable(ctx, chg); err != nil {
			return appliedCount, fmt.Errorf("apply to domain table %s: %w", chg.TableName, err)
		}

		// Record in local changelog
		if err := s.store.ApplyRemoteChange(ctx, chg); err != nil {
			return appliedCount, fmt.Errorf("record remote change: %w", err)
		}

		appliedCount++
	}

	return appliedCount, nil
}

func (s *SyncService) getLatestEntityChange(ctx context.Context, table, entityID string) (*ChangeRecord, error) {
	query := `
	SELECT id, change_id, table_name, entity_id, operation, lamport_time, node_id, payload, created_at
	FROM p2p_changelog
	WHERE table_name = ? AND entity_id = ?
	ORDER BY lamport_time DESC, id DESC
	LIMIT 1;
	`

	var r ChangeRecord
	var createdAt string
	err := s.db.QueryRowContext(ctx, query, table, entityID).Scan(
		&r.ID, &r.ChangeID, &r.TableName, &r.EntityID, &r.Operation,
		&r.LamportTime, &r.NodeID, &r.Payload, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &r, nil
}

func (s *SyncService) applyToDomainTable(ctx context.Context, chg ChangeRecord) error {
	switch chg.TableName {
	case "transactions":
		var tx storage.TransactionRecord
		if err := json.Unmarshal([]byte(chg.Payload), &tx); err != nil {
			return fmt.Errorf("unmarshal transaction: %w", err)
		}

		query := `
		INSERT INTO transactions (
			transaction_id, transaction_type, transaction_api, message_id, request_id,
			payload, provider_id, message_status, error_type, error_code, error_path,
			error_message, req_received_time, additional_data, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(transaction_id, transaction_type, message_id, request_id) DO UPDATE SET
			message_status = excluded.message_status,
			error_type = excluded.error_type,
			error_code = excluded.error_code,
			error_path = excluded.error_path,
			error_message = excluded.error_message,
			payload = excluded.payload,
			additional_data = excluded.additional_data,
			updated_at = CURRENT_TIMESTAMP;
		`
		var addData *string
		if tx.AdditionalData != "" {
			addData = &tx.AdditionalData
		}
		reqTime := tx.ReqReceivedTime
		if reqTime.IsZero() {
			reqTime = time.Now().UTC()
		}

		_, err := s.db.ExecContext(ctx, query,
			tx.TransactionID, tx.TransactionType, tx.TransactionAPI, tx.MessageID, tx.RequestID,
			tx.Payload, tx.ProviderID, tx.MessageStatus, tx.ErrorType, tx.ErrorCode, tx.ErrorPath,
			tx.ErrorMessage, reqTime.Format(time.RFC3339), addData,
		)
		return err

	case "registrations":
		var reg storage.RegistrationRecord
		if err := json.Unmarshal([]byte(chg.Payload), &reg); err != nil {
			return fmt.Errorf("unmarshal registration: %w", err)
		}

		query := `
		INSERT INTO registrations (
			registration_id, ondc_environment, ondc_registry_url, entity_name,
			business_address, gst_details, pan_no, address_signatory, email_id,
			mobile_number, domains_enabled, app_type, subscriber_id, subscriber_url,
			signing_public_key, encr_public_key, creation_time, last_modified_time,
			creation_by, last_modified_by, additional_data
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?, ?)
		ON CONFLICT(registration_id) DO UPDATE SET
			ondc_environment = excluded.ondc_environment,
			ondc_registry_url = excluded.ondc_registry_url,
			entity_name = excluded.entity_name,
			business_address = excluded.business_address,
			gst_details = excluded.gst_details,
			pan_no = excluded.pan_no,
			address_signatory = excluded.address_signatory,
			email_id = excluded.email_id,
			mobile_number = excluded.mobile_number,
			domains_enabled = excluded.domains_enabled,
			app_type = excluded.app_type,
			subscriber_id = excluded.subscriber_id,
			subscriber_url = excluded.subscriber_url,
			signing_public_key = excluded.signing_public_key,
			encr_public_key = excluded.encr_public_key,
			last_modified_time = CURRENT_TIMESTAMP,
			last_modified_by = excluded.last_modified_by,
			additional_data = excluded.additional_data;
		`
		var addData *string
		if reg.AdditionalData != "" {
			addData = &reg.AdditionalData
		}

		_, err := s.db.ExecContext(ctx, query,
			reg.RegistrationID, reg.ONDCEnvironment, reg.ONDCRegistryURL, reg.EntityName,
			reg.BusinessAddress, reg.GSTDetails, reg.PANNo, reg.AddressSignatory, reg.EmailID,
			reg.MobileNumber, reg.DomainsEnabled, reg.AppType, reg.SubscriberID, reg.SubscriberURL,
			reg.SigningPublicKey, reg.EncrPublicKey, reg.CreationBy, reg.LastModifiedBy, addData,
		)
		return err

	default:
		return fmt.Errorf("unknown table %s", chg.TableName)
	}
}

// RegisterHTTPHandlers attaches P2P sync HTTP endpoints to the provided HTTP router.
func (s *SyncService) RegisterHTTPHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/p2p/sync/pull", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid sync request: %v", err), http.StatusBadRequest)
			return
		}

		resp, err := s.HandlePullSync(r.Context(), req)
		if err != nil {
			http.Error(w, fmt.Sprintf("sync pull error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/p2p/sync/push", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req PushSyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid push request: %v", err), http.StatusBadRequest)
			return
		}

		resp, err := s.HandlePushSync(r.Context(), req)
		if err != nil {
			http.Error(w, fmt.Sprintf("sync push error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// PullFromPeer contacts a remote peer via HTTP to fetch changes since a logical clock timestamp.
func (s *SyncService) PullFromPeer(ctx context.Context, peerAddress string, sinceLamportTime uint64) (*SyncResponse, error) {
	url := strings.TrimRight(peerAddress, "/") + "/p2p/sync/pull"
	reqBody, err := json.Marshal(SyncRequest{
		NodeID:           s.nodeID,
		SinceLamportTime: sinceLamportTime,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal pull request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("peer returned status %d: %s", resp.StatusCode, string(body))
	}

	var syncResp SyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return nil, fmt.Errorf("decode sync response: %w", err)
	}

	return &syncResp, nil
}

// PushToPeer sends a batch of changes to a remote peer via HTTP.
func (s *SyncService) PushToPeer(ctx context.Context, peerAddress string, changes []ChangeRecord) (*PushSyncResponse, error) {
	url := strings.TrimRight(peerAddress, "/") + "/p2p/sync/push"
	reqBody, err := json.Marshal(PushSyncRequest{
		NodeID:  s.nodeID,
		Changes: changes,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal push request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute push request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("peer returned status %d: %s", resp.StatusCode, string(body))
	}

	var pushResp PushSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&pushResp); err != nil {
		return nil, fmt.Errorf("decode push response: %w", err)
	}

	return &pushResp, nil
}
