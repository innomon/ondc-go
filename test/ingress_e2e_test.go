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

package test_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"ondc/shared/clients/transactionclient"
	"ondc/shared/storage"
)

const siteVerificationTemplateStr = `<html>
    <head>
        <meta name='ondc-site-verification' content='{{.}}' />
    </head>
    <body>
        ONDC Site Verification Page
    </body>
</html>`

// TestEndToEnd_LocalIngressAndChallengeVerification simulates the full Cloudflare Tunnel
// ingress routing to local services with SQLite persistence and ONDC site verification challenge.
func TestEndToEnd_LocalIngressAndChallengeVerification(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// 1. Setup local sqlite persistence for transaction storage
	sqliteFile := filepath.Join(tmpDir, "local_e2e_transactions.db")
	dbID := fmt.Sprintf("sqlite:%s", sqliteFile)

	txnClient, err := transactionclient.New(ctx, "local-project", "local-instance", dbID)
	if err != nil {
		t.Fatalf("failed to initialize sqlite transaction client: %v", err)
	}

	// 2. Mock Onboarding Service responding to /ondc-site-verification.html and /onboarding/on_subscribe
	challengeToken := "mock-ondc-challenge-signed-token-xyz-12345"
	tmpl := template.Must(template.New("site-verification").Parse(siteVerificationTemplateStr))

	onboardingMux := http.NewServeMux()
	onboardingMux.HandleFunc("/ondc-site-verification.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_ = tmpl.Execute(w, challengeToken)
	})
	onboardingMux.HandleFunc("/onboarding/on_subscribe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"answer": "decrypted-challenge-answer"}`))
	})
	onboardingServer := httptest.NewServer(onboardingMux)
	defer onboardingServer.Close()

	// 3. Mock BAP Service responding to /bap/on_search
	bapMux := http.NewServeMux()
	bapMux.HandleFunc("/bap/on_search", func(w http.ResponseWriter, r *http.Request) {
		// Store inbound transaction into sqlite
		err := txnClient.StoreTransaction(ctx, transactionclient.TransactionData{
			ID:        "e2e-txn-bap-1",
			Type:      "CALLBACK-ACTION",
			API:       "on_search",
			MessageID: "e2e-msg-bap-1",
			Payload:   map[string]any{"status": "received"},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":{"ack":{"status":"ACK"}}}`))
	})
	bapServer := httptest.NewServer(bapMux)
	defer bapServer.Close()

	// 4. Mock BPP Service responding to /bpp/search
	bppMux := http.NewServeMux()
	bppMux.HandleFunc("/bpp/search", func(w http.ResponseWriter, r *http.Request) {
		err := txnClient.StoreTransaction(ctx, transactionclient.TransactionData{
			ID:        "e2e-txn-bpp-1",
			Type:      "REQUEST-ACTION",
			API:       "search",
			MessageID: "e2e-msg-bpp-1",
			Payload:   map[string]any{"intent": "search_items"},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":{"ack":{"status":"ACK"}}}`))
	})
	bppServer := httptest.NewServer(bppMux)
	defer bppServer.Close()

	// 5. Build Cloudflare Tunnel Ingress Router (simulating cloudflared reverse proxy)
	tunnelIngressMux := http.NewServeMux()
	tunnelIngressMux.HandleFunc("/ondc-site-verification.html", func(w http.ResponseWriter, r *http.Request) {
		forwardRequest(w, r, onboardingServer.URL)
	})
	tunnelIngressMux.HandleFunc("/onboarding/", func(w http.ResponseWriter, r *http.Request) {
		forwardRequest(w, r, onboardingServer.URL)
	})
	tunnelIngressMux.HandleFunc("/bap/", func(w http.ResponseWriter, r *http.Request) {
		forwardRequest(w, r, bapServer.URL)
	})
	tunnelIngressMux.HandleFunc("/bpp/", func(w http.ResponseWriter, r *http.Request) {
		forwardRequest(w, r, bppServer.URL)
	})

	tunnelServer := httptest.NewServer(tunnelIngressMux)
	defer tunnelServer.Close()

	// Verify Ingress Route 1: /ondc-site-verification.html
	resp, err := http.Get(tunnelServer.URL + "/ondc-site-verification.html")
	if err != nil {
		t.Fatalf("failed to GET site verification: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for site verification, got %d", resp.StatusCode)
	}
	bodyBuf := make([]byte, 1024)
	n, _ := resp.Body.Read(bodyBuf)
	bodyStr := string(bodyBuf[:n])
	if !strings.Contains(bodyStr, challengeToken) {
		t.Errorf("site verification HTML does not contain expected token %s: %s", challengeToken, bodyStr)
	}

	// Verify Ingress Route 2: /bap/on_search callback receipt
	bapResp, err := http.Post(tunnelServer.URL+"/bap/on_search", "application/json", strings.NewReader(`{"test":true}`))
	if err != nil {
		t.Fatalf("failed to POST /bap/on_search: %v", err)
	}
	defer bapResp.Body.Close()
	if bapResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /bap/on_search, got %d", bapResp.StatusCode)
	}

	// Verify Ingress Route 3: /bpp/search request receipt
	bppResp, err := http.Post(tunnelServer.URL+"/bpp/search", "application/json", strings.NewReader(`{"search":true}`))
	if err != nil {
		t.Fatalf("failed to POST /bpp/search: %v", err)
	}
	defer bppResp.Body.Close()
	if bppResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /bpp/search, got %d", bppResp.StatusCode)
	}

	// Verify Ingress Route 4: Unknown path returns 404 (ingress fallback)
	notFoundResp, err := http.Get(tunnelServer.URL + "/unknown/route")
	if err != nil {
		t.Fatalf("failed to GET unknown route: %v", err)
	}
	defer notFoundResp.Body.Close()
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown route, got %d", notFoundResp.StatusCode)
	}

	// 6. Verify SQLite persistence contains persisted transactions
	repo, err := storage.NewSQLiteRepository(storage.SQLiteOptions{
		DBPath: sqliteFile,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite repository: %v", err)
	}
	defer repo.Close()

	bapTxn, err := repo.GetTransaction(ctx, "e2e-txn-bap-1", "e2e-msg-bap-1")
	if err != nil || bapTxn == nil {
		t.Fatalf("expected BAP transaction in sqlite, got error: %v", err)
	}
	if bapTxn.TransactionAPI != "on_search" {
		t.Errorf("expected BAP transaction API 'on_search', got '%s'", bapTxn.TransactionAPI)
	}

	bppTxn, err := repo.GetTransaction(ctx, "e2e-txn-bpp-1", "e2e-msg-bpp-1")
	if err != nil || bppTxn == nil {
		t.Fatalf("expected BPP transaction in sqlite, got error: %v", err)
	}
	if bppTxn.TransactionAPI != "search" {
		t.Errorf("expected BPP transaction API 'search', got '%s'", bppTxn.TransactionAPI)
	}
}

func forwardRequest(w http.ResponseWriter, r *http.Request, targetServerURL string) {
	targetURL, _ := url.Parse(targetServerURL)
	targetURL.Path = r.URL.Path
	targetURL.RawQuery = r.URL.RawQuery

	req, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for k, v := range r.Header {
		req.Header[k] = v
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}
