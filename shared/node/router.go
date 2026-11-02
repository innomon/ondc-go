package node

import (
	"encoding/json"
	"net/http"
	"text/template"
)

const defaultSiteVerificationHTML = `<html>
    <head>
        <meta name='ondc-site-verification' content='{{.}}' />
    </head>
    <body>
        ONDC Site Verification Page
    </body>
</html>`

var siteVerificationTemplate = template.Must(template.New("site-verification").Parse(defaultSiteVerificationHTML))

// NodeServices holds service handlers and tokens for mounting onto the unified router.
type NodeServices struct {
	OnboardingSiteVerificationToken string
	OnboardingHandler               http.Handler
	BuyerHandler                    http.Handler
	SellerHandler                   http.Handler
	P2PHandler                      http.Handler
	HealthHandler                   http.Handler
}

// NewNodeMux constructs the unified HTTP multiplexer serving onboarding, buyer, seller,
// P2P replication, and health telemetry endpoints.
func NewNodeMux(cfg *NodeConfig, services NodeServices) http.Handler {
	mux := http.NewServeMux()

	// 1. Root & Observability Endpoints
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"service":        "ondc-node",
			"node_id":        cfg.NodeID,
			"mode":           cfg.Mode,
			"domain":         cfg.Domain,
			"buyer_enabled":  cfg.EnableBuyer,
			"seller_enabled": cfg.EnableSeller,
			"p2p_enabled":    cfg.EnableP2P,
			"status":         "running",
		})
	})

	// Health and Status
	if services.HealthHandler != nil {
		mux.Handle("/healthz", services.HealthHandler)
		mux.Handle("/status", services.HealthHandler)
	} else {
		healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "ok",
				"mode":    cfg.Mode,
				"node_id": cfg.NodeID,
			})
		})
		mux.Handle("/healthz", healthHandler)
		mux.Handle("/status", healthHandler)
	}

	// 2. Onboarding & Site Verification
	verificationHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := services.OnboardingSiteVerificationToken
		if token == "" {
			token = "sample-ondc-verification-token"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = siteVerificationTemplate.Execute(w, token)
	})
	mux.Handle("/ondc-site-verification.html", verificationHandler)
	mux.Handle("/.well-known/ondc-site-verification.html", verificationHandler)

	if services.OnboardingHandler != nil {
		mux.Handle("/on_subscribe", services.OnboardingHandler)
	} else {
		mux.HandleFunc("/on_subscribe", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message":{"ack":{"status":"ACK"}}}`))
		})
	}

	// 3. Buyer Platform (BAP) Routes
	bapDisabled := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"Buyer platform is disabled on this node"}`))
	})

	if !cfg.EnableBuyer {
		mux.Handle("/bap/", bapDisabled)
		mux.Handle("/client/", bapDisabled)
	} else if services.BuyerHandler != nil {
		mux.Handle("/bap/", services.BuyerHandler)
		mux.Handle("/client/", services.BuyerHandler)
	} else {
		defaultBuyerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message":{"ack":{"status":"ACK"}}}`))
		})
		mux.Handle("/bap/", defaultBuyerHandler)
		mux.Handle("/client/", defaultBuyerHandler)
	}

	// 4. Seller Platform (BPP) Routes
	bppDisabled := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"Seller platform is disabled on this node"}`))
	})

	if !cfg.EnableSeller {
		mux.Handle("/bpp/", bppDisabled)
	} else if services.SellerHandler != nil {
		mux.Handle("/bpp/", services.SellerHandler)
	} else {
		defaultSellerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message":{"ack":{"status":"ACK"}}}`))
		})
		mux.Handle("/bpp/", defaultSellerHandler)
	}

	// 5. P2P Replication Routes
	p2pDisabled := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"P2P replication is disabled on this node"}`))
	})

	if !cfg.EnableP2P {
		mux.Handle("/p2p/", p2pDisabled)
	} else if services.P2PHandler != nil {
		mux.Handle("/p2p/", services.P2PHandler)
	} else {
		defaultP2PHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","p2p":"active"}`))
		})
		mux.Handle("/p2p/", defaultP2PHandler)
	}

	return mux
}
