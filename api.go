package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"goServer/internal/auth"
	"goServer/internal/database"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
	jwtSecret      string
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerAdminMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	hits := cfg.fileserverHits.Load()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := fmt.Sprintf(`
<html>
	<body>
		<h1>Welcome, Chirpy Admin</h1>
		<p>Chirpy has been visited %d times!</p>
	</body>
</html>
`, hits)
	w.Write([]byte(html))
}

func (cfg *apiConfig) handlerAdminReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if cfg.platform != "dev" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	// Dangerous: delete all users
	err := cfg.dbQueries.DeleteAllUsers(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not delete users")
		return
	}
	cfg.fileserverHits.Store(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("All users deleted and hits reset to 0"))
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	type requestBody struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	var req requestBody
	decoder := http.MaxBytesReader(w, r.Body, 1024)
	err := json.NewDecoder(decoder).Decode(&req)
	if err != nil || req.Email == "" || req.Password == "" {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not hash password")
		return
	}

	dbUser, err := cfg.dbQueries.CreateUser(r.Context(), database.CreateUserParams{
		Email:          req.Email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create user")
		return
	}

	user := User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	type requestBody struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	var req requestBody
	decoder := http.MaxBytesReader(w, r.Body, 1024)
	err := json.NewDecoder(decoder).Decode(&req)
	if err != nil || req.Email == "" || req.Password == "" {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	dbUser, err := cfg.dbQueries.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	if auth.CheckPasswordHash(req.Password, dbUser.HashedPassword) != nil {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	// Access token: 1 hour expiration
	accessToken, err := auth.MakeJWT(dbUser.ID, cfg.jwtSecret, time.Hour)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create JWT")
		return
	}

	// Refresh token: 60 days expiration
	refreshToken, err := auth.MakeRefreshToken()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create refresh token")
		return
	}

	now := time.Now().UTC()
	expiresAt := now.Add(60 * 24 * time.Hour)
	err = cfg.dbQueries.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     refreshToken,
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    dbUser.ID,
		ExpiresAt: expiresAt,
		RevokedAt: sql.NullTime{Valid: false},
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not store refresh token")
		return
	}

	response := struct {
		ID           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refresh_token"`
	}{
		ID:           dbUser.ID,
		CreatedAt:    dbUser.CreatedAt,
		UpdatedAt:    dbUser.UpdatedAt,
		Email:        dbUser.Email,
		Token:        accessToken,
		RefreshToken: refreshToken,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (cfg *apiConfig) handlerCreateChirp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate user
	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil || tokenString == "" {
		respondWithError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
		return
	}
	userID, err := auth.ValidateJWT(tokenString, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid JWT")
		return
	}

	type requestBody struct {
		Body string `json:"body"`
	}
	var req requestBody
	decoder := http.MaxBytesReader(w, r.Body, 1024)
	err = json.NewDecoder(decoder).Decode(&req)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}

	cleaned := cleanProfanity(req.Body)

	chirp, err := cfg.dbQueries.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   cleaned,
		UserID: userID,
	})
	if err != nil {
		log.Printf("CreateChirp error: %v, params: body='%s', user_id='%s'", err, cleaned, userID)
		respondWithError(w, http.StatusInternalServerError, "Could not create chirp")
		return
	}

	response := struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (cfg *apiConfig) handlerGetAllChirps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	chirps, err := cfg.dbQueries.GetAllChirps(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not fetch chirps")
		return
	}

	response := make([]struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}, len(chirps))

	for i, chirp := range chirps {
		response[i].ID = chirp.ID
		response[i].CreatedAt = chirp.CreatedAt
		response[i].UpdatedAt = chirp.UpdatedAt
		response[i].Body = chirp.Body
		response[i].UserID = chirp.UserID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (cfg *apiConfig) handlerGetChirpByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	chirpIDStr := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(chirpIDStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid chirp ID")
		return
	}

	chirp, err := cfg.dbQueries.GetChirpByID(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Chirp not found")
		return
	}

	response := struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (cfg *apiConfig) handlerRefreshToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
		return
	}

	dbToken, err := cfg.dbQueries.GetRefreshToken(r.Context(), tokenString)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid refresh token")
		return
	}
	now := time.Now().UTC()
	if dbToken.ExpiresAt.Before(now) || (dbToken.RevokedAt.Valid && dbToken.RevokedAt.Time.Before(now)) {
		respondWithError(w, http.StatusUnauthorized, "Refresh token expired or revoked")
		return
	}

	// Create new access token
	accessToken, err := auth.MakeJWT(dbToken.UserID, cfg.jwtSecret, time.Hour)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create access token")
		return
	}

	response := struct {
		Token string `json:"token"`
	}{
		Token: accessToken,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (cfg *apiConfig) handlerRevokeRefreshToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
		return
	}

	err = cfg.dbQueries.RevokeRefreshToken(r.Context(), tokenString)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not revoke refresh token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) handlerUpdateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate user
	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
		return
	}
	userID, err := auth.ValidateJWT(tokenString, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid JWT")
		return
	}

	type requestBody struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	var req requestBody
	decoder := http.MaxBytesReader(w, r.Body, 1024)
	err = json.NewDecoder(decoder).Decode(&req)
	if err != nil || req.Email == "" || req.Password == "" {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not hash password")
		return
	}

	dbUser, err := cfg.dbQueries.UpdateUser(r.Context(), database.UpdateUserParams{
		ID:             userID,
		Email:          req.Email,
		HashedPassword: hashedPassword,
		UpdatedAt:      time.Now().UTC(),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not update user")
		return
	}

	user := User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(user)
}

func (cfg *apiConfig) handlerDeleteChirp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate user
	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
		return
	}
	userID, err := auth.ValidateJWT(tokenString, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid JWT")
		return
	}

	chirpIDStr := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(chirpIDStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid chirp ID")
		return
	}

	chirp, err := cfg.dbQueries.GetChirpByID(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Chirp not found")
		return
	}

	if chirp.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You are not the author of this chirp")
		return
	}

	err = cfg.dbQueries.DeleteChirp(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not delete chirp")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
