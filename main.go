package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"

	"github.com/joho/godotenv"
	"github.com/mrbrist/go-http-server/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)

	const filepathRoot = "."
	const filepathAssets = "./assets"
	const port = "8080"

	apiCfg := apiConfig{db: dbQueries}

	httpServeMux := http.NewServeMux()
	httpServeMux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(filepathRoot)))))
	httpServeMux.Handle("/api", apiCfg.middlewareMetricsInc(http.StripPrefix("/api", http.FileServer(http.Dir(filepathAssets)))))
	httpServeMux.HandleFunc("GET /api/healthz", readinessHandler)
	httpServeMux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
	httpServeMux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
	httpServeMux.HandleFunc("POST /api/validate_chirp", apiCfg.validateHandler)

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
	w.WriteHeader(http.StatusOK)

	cfg.fileserverHits.Store(0)
}

func (cfg *apiConfig) validateHandler(w http.ResponseWriter, req *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	type returnVals struct {
		CleanedBody string `json:"cleaned_body,omitempty"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}

	if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	cleaned := CleanString(params.Body)

	respondWithJSON(w, 200, returnVals{
		CleanedBody: cleaned,
	})
}
