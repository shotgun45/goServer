package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"goServer/internal/database"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found or error loading .env file")
	}

	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	log.Printf("DB_URL: %s", dbURL)
	log.Printf("PLATFORM: %s", platform)

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	dbQueries := database.New(db)
	jwtSecret := os.Getenv("JWT_SECRET")
	apiCfg := &apiConfig{
		dbQueries: dbQueries,
		platform:  platform,
		jwtSecret: jwtSecret,
	}

	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("/api/users", apiCfg.handlerCreateUser)
	mux.HandleFunc("/api/login", apiCfg.handlerLogin)
	mux.HandleFunc("/api/chirps", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			apiCfg.handlerGetAllChirps(w, r)
		} else if r.Method == http.MethodPost {
			apiCfg.handlerCreateChirp(w, r)
		} else {
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/chirps/{chirpID}", apiCfg.handlerGetChirpByID)

	fileServer := http.FileServer(http.Dir("."))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", fileServer)))

	mux.HandleFunc("/admin/metrics", apiCfg.handlerAdminMetrics)
	mux.HandleFunc("/admin/reset", apiCfg.handlerAdminReset)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("Starting server on :8080...")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func cleanProfanity(input string) string {
	profaneWords := []string{"kerfuffle", "sharbert", "fornax"}
	words := strings.Split(input, " ")
	for i, word := range words {
		for _, bad := range profaneWords {
			if strings.ToLower(word) == bad {
				words[i] = "****"
			}
		}
	}
	return strings.Join(words, " ")
}
