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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDiscovery_StaticSeeds(t *testing.T) {
	// Setup a mock seed peer HTTP endpoint responding to /p2p/ping
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/p2p/ping" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"seed-node-1","role":"seller","subscriber_id":"seller.example.com"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	registry := NewPeerRegistry(time.Minute)
	discovery := NewDiscoveryService(DiscoveryConfig{
		SelfID:      "local-node",
		StaticSeeds: []string{ts.URL},
		PingTimeout: 500 * time.Millisecond,
		Registry:    registry,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	discovered, err := discovery.ProbeStaticSeeds(ctx)
	if err != nil {
		t.Fatalf("ProbeStaticSeeds failed: %v", err)
	}

	if len(discovered) != 1 {
		t.Fatalf("expected 1 discovered peer, got %d", len(discovered))
	}

	peer := discovered[0]
	if peer.ID != "seed-node-1" {
		t.Errorf("expected peer ID 'seed-node-1', got '%s'", peer.ID)
	}
	if peer.Role != "seller" {
		t.Errorf("expected peer Role 'seller', got '%s'", peer.Role)
	}

	// Verify added to registry
	if _, ok := registry.Get("seed-node-1"); !ok {
		t.Errorf("expected seed-node-1 to be recorded in registry")
	}
}

func TestDiscovery_LocalBroadcastAndReceive(t *testing.T) {
	registry := NewPeerRegistry(time.Minute)

	mockBroadcastChan := make(chan PeerAdvertisement, 5)
	discovery := NewDiscoveryService(DiscoveryConfig{
		SelfID:        "local-node",
		SelfAddress:   "127.0.0.1:9090",
		SelfRole:      "buyer",
		Registry:      registry,
		broadcastChan: mockBroadcastChan,
	})

	// Simulate incoming peer advertisement (e.g. from mDNS / broadcast)
	adv := PeerAdvertisement{
		ID:           "mdns-node-2",
		Address:      "192.168.1.50:9091",
		Role:         "seller",
		SubscriberID: "seller2.local",
	}

	discovery.HandleAdvertisement(adv)

	peer, ok := registry.Get("mdns-node-2")
	if !ok {
		t.Fatalf("expected mdns-node-2 in registry")
	}
	if peer.Address != "192.168.1.50:9091" {
		t.Errorf("expected address 192.168.1.50:9091, got %s", peer.Address)
	}
	if peer.Role != "seller" {
		t.Errorf("expected role seller, got %s", peer.Role)
	}

	// Verify self-advertisement ignored
	discovery.HandleAdvertisement(PeerAdvertisement{
		ID: "local-node",
	})
	if _, ok := registry.Get("local-node"); ok {
		t.Errorf("local node should not register itself")
	}
}
