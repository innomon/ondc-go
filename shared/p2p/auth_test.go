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
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEd25519Handshake_Success(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	challenge, err := GenerateHandshakeChallenge("server-node-1")
	if err != nil {
		t.Fatalf("GenerateHandshakeChallenge failed: %v", err)
	}
	if challenge.ChallengeID == "" || challenge.ServerNodeID != "server-node-1" {
		t.Fatalf("invalid challenge generated: %+v", challenge)
	}

	resp, err := CreateHandshakeResponse(challenge, "client-node-1", "bap.example.com", "key-1", privKey)
	if err != nil {
		t.Fatalf("CreateHandshakeResponse failed: %v", err)
	}
	if resp.PublicKey != base64.StdEncoding.EncodeToString(pubKey) {
		t.Errorf("public key mismatch in handshake response")
	}

	err = VerifyHandshake(challenge, *resp, 30*time.Second)
	if err != nil {
		t.Fatalf("VerifyHandshake failed on valid response: %v", err)
	}
}

func TestEd25519Handshake_Failures(t *testing.T) {
	pubKeyA, privKeyA, _ := ed25519.GenerateKey(rand.Reader)
	_, privKeyB, _ := ed25519.GenerateKey(rand.Reader)

	challenge, _ := GenerateHandshakeChallenge("server-node-1")

	// 1. Signed with wrong key (Private Key B signed, claiming to be Public Key A)
	resp, _ := CreateHandshakeResponse(challenge, "client-node-1", "bap.example.com", "key-1", privKeyB)
	resp.PublicKey = base64.StdEncoding.EncodeToString(pubKeyA) // claim to be A
	if err := VerifyHandshake(challenge, *resp, 30*time.Second); err == nil {
		t.Fatalf("expected verification failure when signature does not match public key")
	}

	// 2. Mismatched challenge ID
	validResp, _ := CreateHandshakeResponse(challenge, "client-node-1", "bap.example.com", "key-1", privKeyA)
	tamperedResp := *validResp
	tamperedResp.ChallengeID = "tampered-challenge-id"
	if err := VerifyHandshake(challenge, tamperedResp, 30*time.Second); err == nil {
		t.Fatalf("expected failure for mismatched challenge ID")
	}

	// 3. Expired timestamp
	expiredChallenge := challenge
	expiredChallenge.Timestamp = time.Now().Add(-10 * time.Minute).Unix()
	expiredResp, _ := CreateHandshakeResponse(expiredChallenge, "client-node-1", "bap.example.com", "key-1", privKeyA)
	expiredResp.Timestamp = time.Now().Add(-10 * time.Minute).Unix()
	if err := VerifyHandshake(expiredChallenge, *expiredResp, 30*time.Second); err == nil {
		t.Fatalf("expected failure for expired timestamp")
	}

	// 4. Invalid base64 public key or signature
	badKeyResp := *validResp
	badKeyResp.PublicKey = "!!!not-base64!!!"
	if err := VerifyHandshake(challenge, badKeyResp, 30*time.Second); err == nil {
		t.Fatalf("expected failure for invalid base64 public key")
	}

	badSigResp := *validResp
	badSigResp.Signature = "!!!not-base64!!!"
	if err := VerifyHandshake(challenge, badSigResp, 30*time.Second); err == nil {
		t.Fatalf("expected failure for invalid base64 signature")
	}
}

func TestP2PRequest_SignAndVerify(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}

	keyLookup := func(nodeID string) (ed25519.PublicKey, error) {
		if nodeID == "node-peer-1" {
			return pubKey, nil
		}
		return nil, ErrPeerNotFound
	}

	body := []byte(`{"message_id":"123","action":"search"}`)
	req := httptest.NewRequest(http.MethodPost, "/p2p/sync/push", bytes.NewReader(body))

	// Sign request
	if err := SignP2PRequest(req, "node-peer-1", privKey, body); err != nil {
		t.Fatalf("SignP2PRequest failed: %v", err)
	}

	// Verify request
	verifiedNodeID, err := VerifyP2PRequest(req, body, keyLookup, 30*time.Second)
	if err != nil {
		t.Fatalf("VerifyP2PRequest failed: %v", err)
	}
	if verifiedNodeID != "node-peer-1" {
		t.Errorf("expected verified nodeID 'node-peer-1', got %s", verifiedNodeID)
	}

	// Fail verification if body is tampered
	tamperedBody := []byte(`{"message_id":"123","action":"tampered"}`)
	_, err = VerifyP2PRequest(req, tamperedBody, keyLookup, 30*time.Second)
	if err == nil {
		t.Fatalf("expected verification failure on tampered body")
	}

	// Fail verification if unknown node
	badNodeReq := httptest.NewRequest(http.MethodPost, "/p2p/sync/push", bytes.NewReader(body))
	_ = SignP2PRequest(badNodeReq, "unknown-node", privKey, body)
	_, err = VerifyP2PRequest(badNodeReq, body, keyLookup, 30*time.Second)
	if err == nil {
		t.Fatalf("expected failure for unknown node")
	}
}
