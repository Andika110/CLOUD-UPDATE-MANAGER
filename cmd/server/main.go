package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"quinos-cloud-update-manager/internal/db"
	"quinos-cloud-update-manager/internal/server"
)

func main() {
	defaultPort := os.Getenv("PORT")
	if defaultPort == "" {
		defaultPort = ":8080"
	} else if defaultPort[0] != ':' {
		defaultPort = ":" + defaultPort
	}

	defaultDB := os.Getenv("DB_PATH")
	if defaultDB == "" {
		defaultDB = "data/quinos_cloud.db"
	}

	defaultStorage := os.Getenv("STORAGE_PATH")
	if defaultStorage == "" {
		defaultStorage = "data/packages"
	}

	defaultHTML := os.Getenv("HTML_PATH")
	if defaultHTML == "" {
		defaultHTML = "web/index.html"
	}

	port := flag.String("port", defaultPort, "HTTP server listener port (e.g. :8080)")
	dbPath := flag.String("db", defaultDB, "SQLite database file path")
	storageDir := flag.String("storage", defaultStorage, "Directory to store uploaded Quinos JAR packages")
	htmlPath := flag.String("html", defaultHTML, "Path to Cloud Admin Dashboard HTML file")
	flag.Parse()

	log.Println("================================================================")
	log.Println("  QUINOS CLOUD UPDATE MANAGER — Central VPS Server v1.0")
	log.Println("  Multi-Outlet, Canary Rollout & Atomic Auto-Rollback Platform")
	log.Println("================================================================")

	database, err := db.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("Fatal: Database initialization error (%s): %v", *dbPath, err)
	}
	defer database.Close()
	log.Printf("[DB] Connected to SQLite database: %s", *dbPath)

	srv := server.NewServer(database, *storageDir, *htmlPath)

	printNetworkAddresses(*port)

	httpServer := &http.Server{
		Addr:    *port,
		Handler: srv,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Fatal: Server listen error: %v", err)
		}
	}()

	log.Printf("[HTTP] Cloud Update Server running on %s", *port)
	log.Printf("[UI] Admin Web Dashboard ready at: http://localhost%s/", *port)
	log.Printf("[API] Agent Heartbeat endpoint: POST http://localhost%s/api/v1/agent/heartbeat", *port)

	// Graceful shutdown on SIGINT or SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("\nReceived shutdown signal. Stopping server gracefully...")
}

func printNetworkAddresses(port string) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return
	}
	fmt.Printf("[Server Access URLs]:\n")
	fmt.Printf("  -> Local:   http://localhost%s/\n", port)
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				fmt.Printf("  -> Network: http://%s%s/\n", ipnet.IP.String(), port)
			}
		}
	}
}
