package node_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ondc/shared/node"
)

func TestNodeMux_Routes(t *testing.T) {
	cfg := &node.NodeConfig{
		Mode:         node.ModeTunnel,
		HTTPPort:     8080,
		Domain:       "testnode.local",
		EnableBuyer:  true,
		EnableSeller: true,
		EnableP2P:    true,
	}

	services := node.NodeServices{
		OnboardingSiteVerificationToken: "ondc-token-xyz-123",
	}

	mux := node.NewNodeMux(cfg, services)

	tests := []struct {
		name           string
		method         string
		url            string
		expectedStatus int
		containsBody   string
	}{
		{
			name:           "Root Node Info",
			method:         http.MethodGet,
			url:            "/",
			expectedStatus: http.StatusOK,
			containsBody:   "testnode.local",
		},
		{
			name:           "Health Check",
			method:         http.MethodGet,
			url:            "/healthz",
			expectedStatus: http.StatusOK,
			containsBody:   `"status":"ok"`,
		},
		{
			name:           "Node Status",
			method:         http.MethodGet,
			url:            "/status",
			expectedStatus: http.StatusOK,
			containsBody:   `"mode":"tunnel"`,
		},
		{
			name:           "Site Verification HTML",
			method:         http.MethodGet,
			url:            "/ondc-site-verification.html",
			expectedStatus: http.StatusOK,
			containsBody:   "ondc-token-xyz-123",
		},
		{
			name:           "Well Known Site Verification",
			method:         http.MethodGet,
			url:            "/.well-known/ondc-site-verification.html",
			expectedStatus: http.StatusOK,
			containsBody:   "ondc-token-xyz-123",
		},
		{
			name:           "Buyer BAP Route",
			method:         http.MethodPost,
			url:            "/bap/on_search",
			expectedStatus: http.StatusOK,
			containsBody:   "",
		},
		{
			name:           "Buyer Client Route",
			method:         http.MethodPost,
			url:            "/client/search",
			expectedStatus: http.StatusOK,
			containsBody:   "",
		},
		{
			name:           "Seller BPP Route",
			method:         http.MethodPost,
			url:            "/bpp/search",
			expectedStatus: http.StatusOK,
			containsBody:   "",
		},
		{
			name:           "P2P Status Route",
			method:         http.MethodGet,
			url:            "/p2p/status",
			expectedStatus: http.StatusOK,
			containsBody:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.url, nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Fatalf("[%s %s] expected status %d, got %d. Body: %s",
					tc.method, tc.url, tc.expectedStatus, rec.Code, rec.Body.String())
			}

			if tc.containsBody != "" && !contains(rec.Body.String(), tc.containsBody) {
				t.Errorf("[%s %s] expected body to contain %q, got %q",
					tc.method, tc.url, tc.containsBody, rec.Body.String())
			}
		})
	}
}

func TestNodeMux_DisabledSubsystems(t *testing.T) {
	cfg := &node.NodeConfig{
		Mode:         node.ModeTunnel,
		HTTPPort:     8080,
		Domain:       "testnode.local",
		EnableBuyer:  false,
		EnableSeller: false,
		EnableP2P:    false,
	}

	mux := node.NewNodeMux(cfg, node.NodeServices{})

	disabledRoutes := []string{
		"/bap/on_search",
		"/client/search",
		"/bpp/search",
		"/p2p/status",
	}

	for _, route := range disabledRoutes {
		t.Run("Disabled "+route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, route, nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("expected 503 Service Unavailable for disabled route %q, got %d", route, rec.Code)
			}
		})
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || searchSubstring(haystack, needle))
}

func searchSubstring(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && stringIndex(s, substr) >= 0)
}

func stringIndex(s, substr string) int {
	n := len(substr)
	for i := 0; i <= len(s)-n; i++ {
		if s[i:i+n] == substr {
			return i
		}
	}
	return -1
}
