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
	"testing"
	"time"
)

func TestPeerRegistry_AddAndGet(t *testing.T) {
	reg := NewPeerRegistry(time.Minute)

	peer := PeerInfo{
		ID:           "node-1",
		Address:      "127.0.0.1:9001",
		SubscriberID: "subscriber.buyer.com",
		Role:         "buyer",
		State:        PeerStateActive,
	}

	if err := reg.Register(peer); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, found := reg.Get("node-1")
	if !found {
		t.Fatalf("expected node-1 to be found")
	}
	if got.Address != peer.Address {
		t.Errorf("expected address %s, got %s", peer.Address, got.Address)
	}
	if got.State != PeerStateActive {
		t.Errorf("expected state %s, got %s", PeerStateActive, got.State)
	}
}

func TestPeerRegistry_UpdateHeartbeat(t *testing.T) {
	reg := NewPeerRegistry(time.Minute)

	peer := PeerInfo{
		ID:      "node-1",
		Address: "127.0.0.1:9001",
		State:   PeerStateDegraded,
	}
	_ = reg.Register(peer)

	now := time.Now()
	reg.Heartbeat("node-1", 45*time.Millisecond)

	got, found := reg.Get("node-1")
	if !found {
		t.Fatalf("expected node-1 to be found")
	}
	if got.State != PeerStateActive {
		t.Errorf("expected state to update to Active after heartbeat, got %s", got.State)
	}
	if got.LastSeen.Before(now) {
		t.Errorf("expected LastSeen to be updated")
	}
	if got.Latency != 45*time.Millisecond {
		t.Errorf("expected latency 45ms, got %v", got.Latency)
	}
}

func TestPeerRegistry_EvictStalePeers(t *testing.T) {
	ttl := 100 * time.Millisecond
	reg := NewPeerRegistry(ttl)

	reg.Register(PeerInfo{
		ID:       "fresh-node",
		Address:  "127.0.0.1:9001",
		LastSeen: time.Now(),
		State:    PeerStateActive,
	})

	reg.Register(PeerInfo{
		ID:       "stale-node",
		Address:  "127.0.0.1:9002",
		LastSeen: time.Now().Add(-200 * time.Millisecond),
		State:    PeerStateActive,
	})

	evicted := reg.EvictStalePeers()
	if len(evicted) != 1 || evicted[0] != "stale-node" {
		t.Fatalf("expected stale-node to be evicted, got %v", evicted)
	}

	if _, found := reg.Get("stale-node"); found {
		t.Errorf("expected stale-node to be removed from registry")
	}
	if _, found := reg.Get("fresh-node"); !found {
		t.Errorf("expected fresh-node to remain in registry")
	}
}

func TestPeerRegistry_ListActive(t *testing.T) {
	reg := NewPeerRegistry(time.Minute)

	reg.Register(PeerInfo{ID: "node-1", State: PeerStateActive})
	reg.Register(PeerInfo{ID: "node-2", State: PeerStateDisconnected})
	reg.Register(PeerInfo{ID: "node-3", State: PeerStateActive})

	active := reg.ListActive()
	if len(active) != 2 {
		t.Fatalf("expected 2 active peers, got %d", len(active))
	}
}

func TestPeerRegistry_Remove(t *testing.T) {
	reg := NewPeerRegistry(time.Minute)

	reg.Register(PeerInfo{ID: "node-1", State: PeerStateActive})
	reg.Remove("node-1")

	if _, found := reg.Get("node-1"); found {
		t.Errorf("expected node-1 to be removed")
	}
}

func TestPeerRegistry_ListAll(t *testing.T) {
	reg := NewPeerRegistry(time.Minute)
	reg.Register(PeerInfo{ID: "node-1", State: PeerStateActive})
	reg.Register(PeerInfo{ID: "node-2", State: PeerStateDisconnected})

	all := reg.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 peers in ListAll, got %d", len(all))
	}
}

