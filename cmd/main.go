package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"pipedrive_api_service/internal/routes"
	"pipedrive_api_service/internal/upstream"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/pipedrive/pipelines", routes.PipelinesHandler)
	mux.HandleFunc("/pipedrive/organizations", routes.OrganizationsHandler)
	mux.HandleFunc("/pipedrive/deals", routes.DealsHandler)

	workers := 4
	queueSize := 1024

	if w := os.Getenv("PIPEDRIVE_BROKER_WORKERS"); w != "" {
		if parsed, err := strconv.Atoi(w); err == nil && parsed > 0 {
			workers = parsed
		}
	}
	if qs := os.Getenv("PIPEDRIVE_BROKER_QUEUE"); qs != "" {
		if parsed, err := strconv.Atoi(qs); err == nil && parsed > 0 {
			queueSize = parsed
		}
	}

	broker := upstream.NewUpstreamBroker(workers, queueSize)
	upstream.SetGlobalBroker(broker)

	server := &http.Server{
		Addr:         ":9010",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Println("listening on :9010")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server listen: %v", err)
		}
	}()

	// wait termination signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutdown signal received")

	// shutdown http server
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	} else {
		log.Println("http server stopped")
	}

	// stop broker
	log.Println("shutting down broker")
	broker.Stop()
	log.Println("broker stopped")
}
