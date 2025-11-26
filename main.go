package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func main() {
	const filepathRoot = "."
	const port = "8080"

	apiCfg := apiConfig{}

	httpServeMux := http.NewServeMux()
	httpServeMux.Handle("/app", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))))
	httpServeMux.HandleFunc("/healthz", readinessHandler)
	httpServeMux.HandleFunc("/metrics", apiCfg.metricsHandler)
	httpServeMux.HandleFunc("/reset", apiCfg.resetHandler)

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
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	hits := cfg.fileserverHits.Load()
	w.Write([]byte("Hits: " + fmt.Sprint(hits)))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)

	cfg.fileserverHits.Store(0)
}
