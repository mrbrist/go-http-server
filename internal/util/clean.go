package util

import (
	"slices"
	"strings"
)

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
