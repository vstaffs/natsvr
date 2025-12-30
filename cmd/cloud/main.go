package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/natsvr/natsvr/internal/cloud"
)

//go:embed all:dist
var webFS embed.FS

func main() {
	// Set embedded frontend
	cloud.WebFS = webFS
	addr := flag.String("addr", ":8080", "Server listen address")
	token := flag.String("token", "", "Authentication token")
	dbPath := flag.String("db", "natsvr.db", "SQLite database path")
	flag.Parse()

	if *token == "" {
		log.Fatal("Token is required. Use -token flag")
	}

	cfg := &cloud.Config{
		Addr:   *addr,
		Token:  *token,
		DBPath: *dbPath,
	}

	server, err := cloud.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down server...")
		server.Shutdown()
		os.Exit(0)
	}()

	log.Printf("Starting natsvr cloud server on %s", *addr)
	if err := server.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

