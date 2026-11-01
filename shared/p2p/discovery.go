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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PeerAdvertisement contains node identity announced over local network / mDNS.
type PeerAdvertisement struct {
	ID           string `json:"id"`
	Address      string `json:"address"`
	SubscriberID string `json:"subscriber_id,omitempty"`
	Role         string `json:"role,omitempty"`
	PublicKey    string `json:"public_key,omitempty"`
}

// DiscoveryConfig holds parameters for hybrid peer discovery.
type DiscoveryConfig struct {
	SelfID        string
	SelfAddress   string
	SelfRole      string
	SubscriberID  string
	StaticSeeds   []string
	PingTimeout   time.Duration
	Registry      *PeerRegistry
	broadcastChan chan PeerAdvertisement
}

// DiscoveryService orchestrates static seed probing and local broadcast handling.
type DiscoveryService struct {
	cfg        DiscoveryConfig
	httpClient *http.Client
}

// NewDiscoveryService creates a new discovery engine.
func NewDiscoveryService(cfg DiscoveryConfig) *DiscoveryService {
	if cfg.PingTimeout <= 0 {
		cfg.PingTimeout = 3 * time.Second
	}
	if cfg.Registry == nil {
		cfg.Registry = NewPeerRegistry(30 * time.Second)
	}
	return &DiscoveryService{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.PingTimeout,
		},
	}
}

// PingResponse represents the payload returned by /p2p/ping.
type PingResponse struct {
	ID           string `json:"id"`
	Role         string `json:"role"`
	SubscriberID string `json:"subscriber_id"`
	PublicKey    string `json:"public_key"`
}

// ProbeStaticSeeds dials configured seed URLs, queries /p2p/ping, and registers active peers.
func (d *DiscoveryService) ProbeStaticSeeds(ctx context.Context) ([]PeerInfo, error) {
	var discovered []PeerInfo

	for _, seed := range d.cfg.StaticSeeds {
		seed = strings.TrimRight(seed, "/")
		pingURL := fmt.Sprintf("%s/p2p/ping", seed)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pingURL, nil)
		if err != nil {
			continue
		}

		start := time.Now()
		resp, err := d.httpClient.Do(req)
		if err != nil {
			continue
		}

		latency := time.Since(start)
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		var pResp PingResponse
		err = json.NewDecoder(resp.Body).Decode(&pResp)
		resp.Body.Close()
		if err != nil || pResp.ID == "" || pResp.ID == d.cfg.SelfID {
			continue
		}

		peer := PeerInfo{
			ID:           pResp.ID,
			Address:      seed,
			SubscriberID: pResp.SubscriberID,
			Role:         pResp.Role,
			PublicKey:    pResp.PublicKey,
			State:        PeerStateActive,
			LastSeen:     time.Now(),
			Latency:      latency,
		}

		if err := d.cfg.Registry.Register(peer); err == nil {
			discovered = append(discovered, peer)
		}
	}

	return discovered, nil
}

// HandleAdvertisement processes an incoming network broadcast or mDNS message.
func (d *DiscoveryService) HandleAdvertisement(adv PeerAdvertisement) {
	if adv.ID == "" || adv.ID == d.cfg.SelfID {
		return
	}

	peer := PeerInfo{
		ID:           adv.ID,
		Address:      adv.Address,
		SubscriberID: adv.SubscriberID,
		Role:         adv.Role,
		PublicKey:    adv.PublicKey,
		State:        PeerStateActive,
		LastSeen:     time.Now(),
	}

	_ = d.cfg.Registry.Register(peer)
}
