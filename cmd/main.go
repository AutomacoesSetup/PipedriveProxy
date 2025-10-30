package main

import (
	"log"
	"net/http"
	"time"

	"pipedrive_api_service/internal/routes"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/pipedrive/pipelines", routes.PipelinesHandler)
	mux.HandleFunc("/pipedrive/organizations", routes.OrganizationsHandler)
	mux.HandleFunc("/pipedrive/deals", routes.DealsHandler)

	server := &http.Server{
		Addr:         ":9010",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Println("listening on :9010")
	log.Fatal(server.ListenAndServe())
}
