package main

import (
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
	"github.com/mrbrist/go-http-server/internal/database"
	"github.com/mrbrist/go-http-server/internal/util"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)

	const filepathRoot = "."
	const filepathAssets = "./assets"
	const port = "8080"

	apiCfg := apiConfig{db: dbQueries, platform: platform}

	httpServeMux := http.NewServeMux()
	httpServeMux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(filepathRoot)))))
	httpServeMux.Handle("/api", apiCfg.middlewareMetricsInc(http.StripPrefix("/api", http.FileServer(http.Dir(filepathAssets)))))
	httpServeMux.HandleFunc("GET /api/healthz", readinessHandler)
	httpServeMux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
	httpServeMux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
	// httpServeMux.HandleFunc("POST /api/validate_chirp", apiCfg.validateHandler)
	httpServeMux.HandleFunc("POST /api/users", apiCfg.createUser)
	httpServeMux.HandleFunc("POST /api/chirps", apiCfg.createChirp)
	httpServeMux.HandleFunc("GET /api/chirps", apiCfg.getAllChirps)
	httpServeMux.HandleFunc("GET /api/chirps/{chirpId}", apiCfg.getChirp)

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
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	user, err := cfg.db.CreateUser(req.Context(), params.Email)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	util.RespondWithJSON(w, 201, User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	})
}

func (cfg *apiConfig) createChirp(w http.ResponseWriter, req *http.Request) {
	type parameters struct {
		Body   string    `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}

	type returnVals struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		util.RespondWithError(w, 500, err.Error())
		return
	}

	if len(params.Body) > 140 {
		util.RespondWithError(w, 400, "Chirp is too long")
		return
	}

	cleaned := util.CleanString(params.Body)

	chirp, err := cfg.db.CreateChirp(req.Context(), database.CreateChirpParams{Body: cleaned, UserID: params.UserID})
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

	chirps, err := cfg.db.GetAllChirps(req.Context())
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
