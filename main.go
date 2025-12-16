package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/mrbrist/go-http-server/internal/auth"
	"github.com/mrbrist/go-http-server/internal/database"
	"github.com/mrbrist/go-http-server/internal/util"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
	jwt_secret     string
	polka_apikey   string
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
	IsChirpyRed  bool      `json:"is_chirpy_red"`
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	jwt_secret := os.Getenv("JWT_SECRET")
	polka_apikey := os.Getenv("POLKA_KEY")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)

	const filepathRoot = "."
	const filepathAssets = "./assets"
	const port = "8080"

	apiCfg := apiConfig{db: dbQueries, platform: platform, jwt_secret: jwt_secret, polka_apikey: polka_apikey}

	httpServeMux := http.NewServeMux()
	httpServeMux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(filepathRoot)))))
	httpServeMux.Handle("/api", apiCfg.middlewareMetricsInc(http.StripPrefix("/api", http.FileServer(http.Dir(filepathAssets)))))

	httpServeMux.HandleFunc("GET /api/healthz", readinessHandler)
	httpServeMux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
	httpServeMux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
	// httpServeMux.HandleFunc("POST /api/validate_chirp", apiCfg.validateHandler)

	httpServeMux.HandleFunc("POST /api/chirps", apiCfg.createChirp)
	httpServeMux.HandleFunc("GET /api/chirps", apiCfg.getAllChirps)
	httpServeMux.HandleFunc("GET /api/chirps/{chirpId}", apiCfg.getChirp)
	httpServeMux.HandleFunc("DELETE /api/chirps/{chirpId}", apiCfg.deleteChirp)

	httpServeMux.HandleFunc("POST /api/login", apiCfg.handleLogin)
	httpServeMux.HandleFunc("POST /api/refresh", apiCfg.refreshToken)
	httpServeMux.HandleFunc("POST /api/revoke", apiCfg.revokeToken)
	httpServeMux.HandleFunc("PUT /api/users", apiCfg.updateUser)
	httpServeMux.HandleFunc("POST /api/users", apiCfg.createUser)

	httpServeMux.HandleFunc("POST /api/polka/webhooks", apiCfg.polkaWebhook)

	server := http.Server{
		Handler: httpServeMux,
		Addr:    ":" + port,
	}

	log.Printf("Serving files on port: %s\n", port)
	log.Fatal(server.ListenAndServe())
}

func readinessHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	hits := cfg.fileserverHits.Load()
	w.Write([]byte(fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", hits)))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, req *http.Request) {
	if cfg.platform != "dev" {
		w.WriteHeader(403)
		return
	}

	w.WriteHeader(http.StatusOK)
	err := cfg.db.ResetUsers(req.Context())
	if err != nil {
		log.Fatal(err)
		return
	}

	cfg.fileserverHits.Store(0)
}

func (cfg *apiConfig) createUser(w http.ResponseWriter, req *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	hash, err := auth.HashPassword(params.Password)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	user, err := cfg.db.CreateUser(req.Context(), database.CreateUserParams{Email: params.Email, HashedPassword: hash})
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	util.RespondWithJSON(w, 201, User{
		ID:          user.ID,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		Email:       user.Email,
		IsChirpyRed: user.IsChirpyRed.Bool,
	})
}

func (cfg *apiConfig) createChirp(w http.ResponseWriter, req *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	type returnVals struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}
	uuid, err := auth.ValidateJWT(token, cfg.jwt_secret)
	if err != nil {
		util.RespondWithError(w, 401, err.Error())
		return
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	// if uuid != params.UserID {
	// 	if err != nil {
	// 		util.RespondWithError(w, 401, "Unauthorized")
	// 		return
	// 	}
	// }

	if len(params.Body) > 140 {
		util.RespondWithError(w, 400, "Chirp is too long")
		return
	}

	cleaned := util.CleanString(params.Body)

	chirp, err := cfg.db.CreateChirp(req.Context(), database.CreateChirpParams{Body: cleaned, UserID: uuid})
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	util.RespondWithJSON(w, 201, returnVals(chirp))
}

func (cfg *apiConfig) getAllChirps(w http.ResponseWriter, req *http.Request) {
	type returnVals struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	author := req.URL.Query().Get("author_id")
	sort := req.URL.Query().Get("sort")

	if author != "" {
		uuid, err := uuid.Parse(author)
		if err != nil {
			util.RespondWithError(w, 500, err.Error())
			return
		}
		chirps, err := cfg.db.GetAllChirpsForUser(req.Context(), database.GetAllChirpsForUserParams{
			UserID:  uuid,
			Column2: sort,
		})
		if err != nil {
			util.RespondWithError(w, 500, err.Error())
			return
		}

		newSlice := make([]returnVals, len(chirps))
		for i, c := range chirps {
			newSlice[i] = returnVals(c)
		}

		util.RespondWithJSON(w, 200, newSlice)
		return
	}

	chirps, err := cfg.db.GetAllChirps(req.Context(), sort)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	newSlice := make([]returnVals, len(chirps))
	for i, c := range chirps {
		newSlice[i] = returnVals(c)
	}

	util.RespondWithJSON(w, 200, newSlice)
}

func (cfg *apiConfig) getChirp(w http.ResponseWriter, req *http.Request) {
	type returnVals struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	uuid, err := uuid.Parse(req.PathValue("chirpId"))
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}
	fmt.Println(uuid)
	chirp, err := cfg.db.GetChirp(req.Context(), uuid)
	if err != nil {
		util.RespondWithError(w, 404, err.Error())
		return
	}

	util.RespondWithJSON(w, 200, returnVals(chirp))
}

func (cfg *apiConfig) handleLogin(w http.ResponseWriter, req *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	user, err := cfg.db.GetUser(context.Background(), params.Email)
	if err != nil {
		util.RespondWithError(w, 401, "Incorrect email or password")
		return
	}

	match, err := auth.CheckPasswordHash(params.Password, user.HashedPassword)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	token, err := auth.MakeJWT(user.ID, cfg.jwt_secret, time.Duration(3600)*time.Second)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	refresh_token, _ := auth.MakeRefreshToken()

	_, err = cfg.db.NewToken(context.Background(), database.NewTokenParams{
		Token:  refresh_token,
		UserID: user.ID,
	})
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	if match {
		util.RespondWithJSON(w, 200, User{
			ID:           user.ID,
			CreatedAt:    user.CreatedAt,
			UpdatedAt:    user.UpdatedAt,
			Email:        user.Email,
			Token:        token,
			RefreshToken: refresh_token,
			IsChirpyRed:  user.IsChirpyRed.Bool,
		})
	} else {
		util.RespondWithError(w, 401, "Incorrect email or password")
		return
	}
}

func (cfg *apiConfig) refreshToken(w http.ResponseWriter, req *http.Request) {
	type returnVals struct {
		Token string `json:"token"`
	}

	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}
	refresh_token, err := cfg.db.GetToken(context.Background(), token)
	if err != nil {
		util.RespondWithError(w, 401, err.Error())
		return
	}
	if time.Until(refresh_token.ExpiresAt) < 0 {
		util.RespondWithError(w, 401, "token expired")
		return
	}
	if refresh_token.RevokedAt.Valid && !refresh_token.RevokedAt.Time.IsZero() {
		util.RespondWithError(w, 401, "token revoked")
		return
	}

	user_id, err := cfg.db.GetUserFromRefreshToken(context.Background(), token)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	access_token, err := auth.MakeJWT(user_id, cfg.jwt_secret, time.Duration(3600)*time.Second)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	util.RespondWithJSON(w, 200, returnVals{
		Token: access_token,
	})
}

func (cfg *apiConfig) revokeToken(w http.ResponseWriter, req *http.Request) {
	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}
	err = cfg.db.RevokeToken(context.Background(), token)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (cfg *apiConfig) updateUser(w http.ResponseWriter, req *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	type response struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		util.RespondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		util.RespondWithError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwt_secret)
	if err != nil {
		util.RespondWithError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		util.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updatedUser, err := cfg.db.UpdateUser(req.Context(), database.UpdateUserParams{
		ID:             userID,
		Email:          params.Email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		util.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	util.RespondWithJSON(w, http.StatusOK, response{
		ID:        updatedUser.ID,
		CreatedAt: updatedUser.CreatedAt,
		UpdatedAt: updatedUser.UpdatedAt,
		Email:     updatedUser.Email,
	})
}

func (cfg *apiConfig) deleteChirp(w http.ResponseWriter, req *http.Request) {
	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		util.RespondWithError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwt_secret)
	if err != nil {
		util.RespondWithError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	uuid, err := uuid.Parse(req.PathValue("chirpId"))
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	chirp, err := cfg.db.GetChirp(context.Background(), uuid)
	if err != nil {
		util.RespondWithError(w, 404, err.Error())
		return
	}

	if chirp.UserID != userID {
		util.RespondWithError(w, 403, "you do not own this chirp")
		return
	}

	err = cfg.db.DeleteChrip(context.Background(), uuid)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	w.WriteHeader(204)
}

func (cfg *apiConfig) polkaWebhook(w http.ResponseWriter, req *http.Request) {
	type parameters struct {
		Event string `json:"event"`
		Data  struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		util.RespondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	key, err := auth.GetAPIKey(req.Header)
	if err != nil {
		util.RespondWithError(w, 401, err.Error())
		return
	}

	if key != cfg.polka_apikey {
		w.WriteHeader(401)
		return
	}

	if params.Event != "user.upgraded" {
		w.WriteHeader(204)
		return
	}

	uuid, err := uuid.Parse(params.Data.UserID)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	err = cfg.db.UpgradeUser(context.Background(), uuid)
	if err != nil {
		util.RespondWithError(w, 404, err.Error())
		return
	}

	w.WriteHeader(204)
}
