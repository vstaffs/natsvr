package main

import (
	"embed"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/natsvr/natsvr/internal/cloud"
	"gopkg.in/yaml.v3"
)

//go:embed all:dist
var webFS embed.FS

// ConfigFile represents the config file structure (JSON or YAML)
type ConfigFile struct {
	Addr       string `json:"addr" yaml:"addr"`
	Token      string `json:"token" yaml:"token"`
	AdminToken string `json:"admin_token" yaml:"admin_token"`
	DBPath     string `json:"db" yaml:"db"`
	DataDir    string `json:"data_dir" yaml:"data_dir"`
}

func main() {
	// Set embedded frontend
	cloud.WebFS = webFS
	configPath := flag.String("config", "", "Path to config file (JSON or YAML)")
	addr := flag.String("addr", ":8080", "Server listen address")
	token := flag.String("token", "", "Authentication token")
	dbPath := flag.String("db", "natsvr.db", "SQLite database path")
	devMode := flag.Bool("dev", false, "Enable development mode (proxy frontend to Vite dev server)")
	devURL := flag.String("dev-url", "http://localhost:5173", "Vite dev server URL")
	flag.Parse()

	// Start with defaults/flags
	cfg := &cloud.Config{
		Addr:    *addr,
		Token:   *token,
		DBPath:  *dbPath,
		DevMode: *devMode,
		DevURL:  *devURL,
	}

	// If config file is provided, load it (overrides defaults but not explicit flags)
	if *configPath != "" {
		fileCfg, err := loadConfigFile(*configPath)
		if err != nil {
			log.Fatalf("Failed to load config file: %v", err)
		}
		// Apply config file values (only if not overridden by flags)
		if fileCfg.Addr != "" && *addr == ":8080" {
			cfg.Addr = fileCfg.Addr
		}
		// Support both "token" and "admin_token" in config
		configToken := fileCfg.Token
		if configToken == "" {
			configToken = fileCfg.AdminToken
		}
		if configToken != "" && *token == "" {
			cfg.Token = configToken
		}
		// Support both "db" and "data_dir" in config
		configDB := fileCfg.DBPath
		if configDB == "" && fileCfg.DataDir != "" {
			configDB = filepath.Join(fileCfg.DataDir, "natsvr.db")
		}
		if configDB != "" && *dbPath == "natsvr.db" {
			cfg.DBPath = configDB
		}
	}

	if cfg.Token == "" {
		log.Fatal("Token is required. Use -token flag or config file")
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

	log.Printf("Starting natsvr cloud server on %s", cfg.Addr)
	if err := server.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func loadConfigFile(path string) (*ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg ConfigFile
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	default:
		// JSON (strip comments first)
		data = stripJSONComments(data)
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

func stripJSONComments(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		result = append(result, line)
	}
	return []byte(strings.Join(result, "\n"))
}
