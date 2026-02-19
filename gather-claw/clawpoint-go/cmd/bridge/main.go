package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"clawpoint-go/connectors"
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

	fmt.Println("ClawPoint Bridge starting...")
	fmt.Printf("  ADK: %s\n", adkURL)

	mb := connectors.NewMatterbridgeConnector(adkURL)
	go func() {
		if err := mb.Start(ctx); err != nil {
			log.Printf("bridge error: %v", err)
		}
	}()

	<-sigChan
	fmt.Println("\nShutting down...")
	cancel()
}
