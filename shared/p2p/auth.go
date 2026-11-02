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
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

var (
	// ErrPeerNotFound indicates the requested peer node public key could not be resolved.
	ErrPeerNotFound = errors.New("peer not found")

	// ErrInvalidSignature indicates signature verification failed.
	ErrInvalidSignature = errors.New("invalid cryptographic signature")

	// ErrExpiredChallenge indicates the challenge or request timestamp is outside the allowed skew window.
	ErrExpiredChallenge = errors.New("challenge or timestamp has expired")

	// ErrInvalidChallenge indicates the challenge parameter does not match expected values.
	ErrInvalidChallenge = errors.New("invalid handshake challenge")
)

// HandshakeChallenge contains server nonce parameters for peer authentication.
type HandshakeChallenge struct {
	ChallengeID  string `json:"challenge_id"`
	ServerNodeID string `json:"server_node_id"`
	Timestamp    int64  `json:"timestamp"`
}

// HandshakeResponse encapsulates the client signature and public key for mutual authentication.
type HandshakeResponse struct {
	ChallengeID  string `json:"challenge_id"`
	ClientNodeID string `json:"client_node_id"`
	SubscriberID string `json:"subscriber_id"`
	KeyID        string `json:"key_id"`
	PublicKey    string `json:"public_key"` // base64 encoded Ed25519 public key
	Signature    string `json:"signature"`  // base64 encoded Ed25519 signature
	Timestamp    int64  `json:"timestamp"`
}

// GenerateHandshakeChallenge generates a random challenge nonce for authenticating incoming peers.
func GenerateHandshakeChallenge(serverNodeID string) (HandshakeChallenge, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return HandshakeChallenge{}, fmt.Errorf("generate nonce: %w", err)
	}

	return HandshakeChallenge{
		ChallengeID:  hex.EncodeToString(nonce),
		ServerNodeID: serverNodeID,
		Timestamp:    time.Now().Unix(),
	}, nil
}

// CreateHandshakeResponse signs a server handshake challenge using the node's Ed25519 private key.
func CreateHandshakeResponse(challenge HandshakeChallenge, clientNodeID, subscriberID, keyID string, privateKey ed25519.PrivateKey) (*HandshakeResponse, error) {
	ts := time.Now().Unix()
	pubKey := privateKey.Public().(ed25519.PublicKey)

	signingData := fmt.Sprintf("%s:%s:%s:%d", challenge.ChallengeID, clientNodeID, challenge.ServerNodeID, ts)
	sig := ed25519.Sign(privateKey, []byte(signingData))

	return &HandshakeResponse{
		ChallengeID:  challenge.ChallengeID,
		ClientNodeID: clientNodeID,
		SubscriberID: subscriberID,
		KeyID:        keyID,
		PublicKey:    base64.StdEncoding.EncodeToString(pubKey),
		Signature:    base64.StdEncoding.EncodeToString(sig),
		Timestamp:    ts,
	}, nil
}

// VerifyHandshake verifies that a handshake response satisfies the challenge and was signed by the claimed public key.
func VerifyHandshake(challenge HandshakeChallenge, resp HandshakeResponse, maxClockSkew time.Duration) error {
	if resp.ChallengeID != challenge.ChallengeID {
		return fmt.Errorf("%w: challenge ID mismatch", ErrInvalidChallenge)
	}

	now := time.Now().Unix()
	skewSeconds := int64(maxClockSkew.Seconds())
	if resp.Timestamp < now-skewSeconds || resp.Timestamp > now+skewSeconds {
		return fmt.Errorf("%w: response timestamp skew too large", ErrExpiredChallenge)
	}
	if challenge.Timestamp < now-skewSeconds || challenge.Timestamp > now+skewSeconds {
		return fmt.Errorf("%w: challenge timestamp expired", ErrExpiredChallenge)
	}

	pubKeyBytes, err := base64.StdEncoding.DecodeString(resp.PublicKey)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: invalid public key bytes", ErrInvalidSignature)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(resp.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("%w: invalid signature bytes", ErrInvalidSignature)
	}

	signingData := fmt.Sprintf("%s:%s:%s:%d", challenge.ChallengeID, resp.ClientNodeID, challenge.ServerNodeID, resp.Timestamp)
	if !ed25519.Verify(pubKeyBytes, []byte(signingData), sigBytes) {
		return fmt.Errorf("%w: Ed25519 verification failed", ErrInvalidSignature)
	}

	return nil
}

// SignP2PRequest attaches cryptographic authentication headers to an HTTP request.
func SignP2PRequest(req *http.Request, nodeID string, privateKey ed25519.PrivateKey, body []byte) error {
	ts := time.Now().Unix()
	bodyHash := sha256.Sum256(body)
	signingPayload := fmt.Sprintf("%s:%s:%d:%s", req.Method, req.URL.Path, ts, hex.EncodeToString(bodyHash[:]))

	sig := ed25519.Sign(privateKey, []byte(signingPayload))

	req.Header.Set("X-P2P-Node-ID", nodeID)
	req.Header.Set("X-P2P-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-P2P-Signature", base64.StdEncoding.EncodeToString(sig))
	return nil
}

// VerifyP2PRequest validates the cryptographic headers of an incoming P2P HTTP request.
func VerifyP2PRequest(req *http.Request, body []byte, keyLookup func(string) (ed25519.PublicKey, error), maxClockSkew time.Duration) (string, error) {
	nodeID := req.Header.Get("X-P2P-Node-ID")
	tsStr := req.Header.Get("X-P2P-Timestamp")
	sigB64 := req.Header.Get("X-P2P-Signature")

	if nodeID == "" || tsStr == "" || sigB64 == "" {
		return "", fmt.Errorf("%w: missing required p2p auth headers", ErrInvalidSignature)
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return "", fmt.Errorf("%w: invalid timestamp header", ErrInvalidSignature)
	}

	now := time.Now().Unix()
	skewSeconds := int64(maxClockSkew.Seconds())
	if ts < now-skewSeconds || ts > now+skewSeconds {
		return "", fmt.Errorf("%w: timestamp outside allowed skew", ErrExpiredChallenge)
	}

	pubKey, err := keyLookup(nodeID)
	if err != nil {
		return "", fmt.Errorf("lookup peer key for %s: %w", nodeID, err)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return "", fmt.Errorf("%w: invalid signature format", ErrInvalidSignature)
	}

	bodyHash := sha256.Sum256(body)
	signingPayload := fmt.Sprintf("%s:%s:%d:%s", req.Method, req.URL.Path, ts, hex.EncodeToString(bodyHash[:]))

	if !ed25519.Verify(pubKey, []byte(signingPayload), sigBytes) {
		return "", fmt.Errorf("%w: request signature mismatch", ErrInvalidSignature)
	}

	return nodeID, nil
}
