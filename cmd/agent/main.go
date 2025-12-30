package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/natsvr/natsvr/internal/agent"
)

func main() {
	serverURL := flag.String("server", "ws://localhost:8080/ws", "Cloud server WebSocket URL")
	token := flag.String("token", "", "Authentication token")
	name := flag.String("name", "", "Agent name")
	flag.Parse()

	if *token == "" {
		log.Fatal("Token is required. Use -token flag")
	}

	if *name == "" {
		hostname, _ := os.Hostname()
		*name = hostname
	}

	cfg := &agent.Config{
		ServerURL: *serverURL,
		Token:     *token,
		Name:      *name,
	}

	client, err := agent.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create agent client: %v", err)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down agent...")
		client.Shutdown()
		os.Exit(0)
	}()

	log.Printf("Starting natsvr agent '%s', connecting to %s", *name, *serverURL)
	client.Run()
}

