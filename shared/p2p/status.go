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
	"encoding/json"
	"net/http"
	"time"
)

// NodeStatus encapsulates operational and synchronization metrics for an ONDC P2P node.
type NodeStatus struct {
	NodeID             string     `json:"node_id"`
	SubscriberID       string     `json:"subscriber_id"`
	Role               string     `json:"role"`
	CurrentLamportTime uint64     `json:"current_lamport_time"`
	TotalChanges       int64      `json:"total_changes"`
	ActivePeers        []PeerInfo `json:"active_peers"`
	UptimeSeconds      int64      `json:"uptime_seconds"`
}

// StatusConfig configures the HTTP status handler.
type StatusConfig struct {
	NodeID       string
	SubscriberID string
	Role         string
	Clock        *LamportClock
	Store        *ChangeLogStore
	Registry     *PeerRegistry
	StartTime    time.Time
}

// StatusHandler serves HTTP GET requests for /p2p/status.
type StatusHandler struct {
	cfg StatusConfig
}

// NewStatusHandler constructs a new StatusHandler.
func NewStatusHandler(cfg StatusConfig) *StatusHandler {
	if cfg.StartTime.IsZero() {
		cfg.StartTime = time.Now()
	}
	return &StatusHandler{cfg: cfg}
}

// ServeHTTP handles /p2p/status requests and returns JSON status.
func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var totalChanges int64
	if h.cfg.Store != nil && h.cfg.Store.db != nil {
		_ = h.cfg.Store.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM p2p_changelog;").Scan(&totalChanges)
	}

	var activePeers []PeerInfo
	if h.cfg.Registry != nil {
		activePeers = h.cfg.Registry.ListActive()
	}

	var currentClock uint64
	if h.cfg.Clock != nil {
		currentClock = h.cfg.Clock.Current()
	}

	uptime := int64(time.Since(h.cfg.StartTime).Seconds())

	status := NodeStatus{
		NodeID:             h.cfg.NodeID,
		SubscriberID:       h.cfg.SubscriberID,
		Role:               h.cfg.Role,
		CurrentLamportTime: currentClock,
		TotalChanges:       totalChanges,
		ActivePeers:        activePeers,
		UptimeSeconds:      uptime,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}
