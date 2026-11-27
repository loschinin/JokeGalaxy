package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/loschinin/JokeGalaxy/backend/internal/app"
)

func main() {
	server, err := app.NewFromEnv()
	if err != nil { log.Fatalf("initialize JokeGalaxy: %v", err) }
	defer server.Close()

	port := os.Getenv("PORT")
	if port == "" { port = "8080" }
	address := ":" + port
	log.Printf("JokeGalaxy backend listening on %s", address)
	httpServer := &http.Server{
		Addr: address,
		Handler: server.Handler(),
		ReadHeaderTimeout: 8 * time.Second,
		ReadTimeout: 20 * time.Second,
		WriteTimeout: 0, // SSE generation streams may last longer than a normal request.
		IdleTimeout: 60 * time.Second,
	}
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed { log.Fatalf("HTTP server: %v", err) }
}
