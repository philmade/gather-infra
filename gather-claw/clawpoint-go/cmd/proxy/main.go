// clawpoint-proxy — public-facing HTTP server for claw subdomains.
//
// Serves static files from /app/public/ and reverse-proxies /api/* to the
// internal ADK server. This is what {name}.gather.is routes to.
//
// Build: cd clawpoint-go && go build -o clawpoint-proxy ./cmd/proxy

package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	listenAddr := getEnv("PROXY_ADDR", ":8080")
	adkAddr := getEnv("ADK_INTERNAL", "http://127.0.0.1:8081")
	bridgeAddr := getEnv("BRIDGE_INTERNAL", "http://127.0.0.1:8082")
	publicDir := getEnv("PUBLIC_DIR", "/app/public")

	// Parse ADK backend URL
	adkURL, err := url.Parse(adkAddr)
	if err != nil {
		log.Fatalf("invalid ADK_INTERNAL url: %v", err)
	}

	// Parse bridge backend URL
	bridgeURL, err := url.Parse(bridgeAddr)
	if err != nil {
		log.Fatalf("invalid BRIDGE_INTERNAL url: %v", err)
	}

	// Reverse proxy for /api/* routes → ADK
	proxy := httputil.NewSingleHostReverseProxy(adkURL)

	// Reverse proxy for /msg → bridge middleware
	bridgeProxy := httputil.NewSingleHostReverseProxy(bridgeURL)

	// Streaming proxy with immediate flush for SSE
	streamBridgeProxy := httputil.NewSingleHostReverseProxy(bridgeURL)
	streamBridgeProxy.FlushInterval = -1 // flush immediately for SSE

	mux := http.NewServeMux()

	// /msg/stream → bridge /stream (SSE streaming pipeline)
	mux.HandleFunc("/msg/stream", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/stream"
		streamBridgeProxy.ServeHTTP(w, r)
	})

	// /msg → bridge middleware (unified message pipeline)
	mux.HandleFunc("/msg", func(w http.ResponseWriter, r *http.Request) {
		// Rewrite path: /msg → /message (bridge endpoint)
		r.URL.Path = "/message"
		bridgeProxy.ServeHTTP(w, r)
	})

	// API routes → ADK (direct, no middleware — dev/debug)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})

	// Activity JSON with CORS for local dev
	mux.HandleFunc("/activity.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, publicDir+"/activity.json")
	})

	// Everything else → static files
	fs := http.FileServer(http.Dir(publicDir))
	mux.Handle("/", fs)

	fmt.Printf("ClawPoint proxy starting...\n")
	fmt.Printf("  Listen:  %s\n", listenAddr)
	fmt.Printf("  ADK:     %s\n", adkAddr)
	fmt.Printf("  Bridge:  %s\n", bridgeAddr)
	fmt.Printf("  Public:  %s\n", publicDir)

	server := &http.Server{Addr: listenAddr, Handler: mux}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		fmt.Println("\nProxy shutting down...")
		server.Close()
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("proxy: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
