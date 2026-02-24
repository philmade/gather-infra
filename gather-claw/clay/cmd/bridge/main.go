package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"clay/core/connectors"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	adkURL := os.Getenv("ADK_URL")
	if adkURL == "" {
		adkURL = "http://127.0.0.1:8080"
	}

	httpAddr := os.Getenv("BRIDGE_ADDR")
	if httpAddr == "" {
		httpAddr = ":8082"
	}

	fmt.Println("ClawPoint Bridge starting...")
	fmt.Printf("  ADK:  %s\n", adkURL)
	fmt.Printf("  HTTP: %s\n", httpAddr)

	mb := connectors.NewMatterbridgeConnector(adkURL)

	// Internal heartbeat (agent-controlled interval)
	go mb.StartHeartbeat(ctx)

	// Matterbridge stream reader (Telegram)
	go func() {
		if err := mb.Start(ctx); err != nil {
			log.Printf("matterbridge stream error: %v", err)
		}
	}()

	// HTTP server (Gather UI + any external callers)
	go func() {
		if err := mb.ServeHTTP(ctx, httpAddr); err != nil {
			log.Printf("bridge HTTP error: %v", err)
		}
	}()

	<-sigChan
	fmt.Println("\nShutting down...")
	cancel()
}
