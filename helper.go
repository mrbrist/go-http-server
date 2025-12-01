package main

import (
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"strings"
)

type errorRes struct {
	Error string `json:"error,omitempty"`
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	res := errorRes{Error: msg}
	dat, err := json.Marshal(res)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func CleanString(s string) string {
	bannedWords := []string{"kerfuffle", "sharbert", "fornax"}
	split := strings.Split(s, " ")
	for i, w := range split {
		if slices.Contains(bannedWords, strings.ToLower(w)) {
			split[i] = "****"
		}
	}

	return strings.Join(split, " ")
}
