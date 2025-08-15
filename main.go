package main

import (
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	// Example route

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("Starting server on :8080...")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
