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
	"errors"
	"sync"
	"time"
)

// PeerState represents the operational status of a peer in the network.
type PeerState string

const (
	PeerStateActive       PeerState = "active"
	PeerStateDegraded     PeerState = "degraded"
	PeerStateDisconnected PeerState = "disconnected"
)

// PeerInfo holds metadata and connectivity stats for a discovered peer.
type PeerInfo struct {
	ID           string        `json:"id"`
	Address      string        `json:"address"`
	SubscriberID string        `json:"subscriber_id,omitempty"`
	Role         string        `json:"role,omitempty"` // buyer, seller, gateway
	PublicKey    string        `json:"public_key,omitempty"`
	State        PeerState     `json:"state"`
	LastSeen     time.Time     `json:"last_seen"`
	Latency      time.Duration `json:"latency"`
	JoinedAt     time.Time     `json:"joined_at"`
}

// PeerRegistry manages active peer connections and liveness timeouts.
type PeerRegistry struct {
	mu      sync.RWMutex
	peers   map[string]PeerInfo
	peerTTL time.Duration
}

// NewPeerRegistry creates a new thread-safe peer registry with specified heartbeat TTL.
func NewPeerRegistry(peerTTL time.Duration) *PeerRegistry {
	if peerTTL <= 0 {
		peerTTL = 30 * time.Second
	}
	return &PeerRegistry{
		peers:   make(map[string]PeerInfo),
		peerTTL: peerTTL,
	}
}

// Register adds or updates a peer in the registry.
func (r *PeerRegistry) Register(peer PeerInfo) error {
	if peer.ID == "" {
		return errors.New("peer ID cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if peer.LastSeen.IsZero() {
		peer.LastSeen = now
	}
	if peer.JoinedAt.IsZero() {
		peer.JoinedAt = now
	}
	if peer.State == "" {
		peer.State = PeerStateActive
	}

	r.peers[peer.ID] = peer
	return nil
}

// Get returns the peer info by peer ID if present.
func (r *PeerRegistry) Get(id string) (PeerInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	peer, ok := r.peers[id]
	return peer, ok
}

// Heartbeat refreshes the peer's last seen timestamp and records latency.
func (r *PeerRegistry) Heartbeat(id string, latency time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	peer, ok := r.peers[id]
	if !ok {
		return false
	}
	peer.LastSeen = time.Now()
	peer.Latency = latency
	peer.State = PeerStateActive
	r.peers[id] = peer
	return true
}

// Remove deletes a peer from the registry.
func (r *PeerRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, id)
}

// ListAll returns a slice copy of all registered peers.
func (r *PeerRegistry) ListAll() []PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]PeerInfo, 0, len(r.peers))
	for _, p := range r.peers {
		res = append(res, p)
	}
	return res
}

// ListActive returns all peers currently marked Active.
func (r *PeerRegistry) ListActive() []PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]PeerInfo, 0)
	for _, p := range r.peers {
		if p.State == PeerStateActive {
			res = append(res, p)
		}
	}
	return res
}

// EvictStalePeers checks peers against peerTTL and removes peers whose LastSeen is expired.
// Returns a slice of evicted peer IDs.
func (r *PeerRegistry) EvictStalePeers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	var evicted []string
	for id, peer := range r.peers {
		if now.Sub(peer.LastSeen) > r.peerTTL {
			delete(r.peers, id)
			evicted = append(evicted, id)
		}
	}
	return evicted
}
