package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const userAgent = "audiolibrix-abs/1.0 (Audiobookshelf metadata provider)"

type config struct {
	port      string
	authToken string
	search    SearchOptions
}

func loadConfig() config {
	cfg := config{
		port:      env("PORT", "3001"),
		authToken: os.Getenv("AUTH_TOKEN"),
		search: SearchOptions{
			MaxResults:  envInt("MAX_RESULTS", 10),
			MaxFetch:    envInt("MAX_FETCH", 30),
			Concurrency: envInt("CONCURRENCY", 5),
			AddLink:     strings.EqualFold(env("ADD_LINK_TO_DESCRIPTION", "true"), "true"),
		},
	}
	if langs := strings.ToLower(env("LANGUAGES", "cs,sk")); langs != "all" {
		cfg.search.Languages = map[string]bool{}
		for _, l := range strings.Split(langs, ",") {
			cfg.search.Languages[strings.TrimSpace(l)] = true
		}
	}
	return cfg
}

func main() {
	cfg := loadConfig()
	client := NewClient(userAgent)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || (cfg.authToken != "" && auth != cfg.authToken) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("query"))
		if query == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query parameter is required"})
			return
		}
		books, err := client.Search(r.Context(), query, r.URL.Query().Get("author"), cfg.search)
		if err != nil {
			slog.Error("search failed", "query", query, "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		slog.Info("search done", "query", query, "matches", len(books))
		writeJSON(w, http.StatusOK, map[string][]Book{"matches": books})
	})

	slog.Info("listening", "port", cfg.port, "languages", env("LANGUAGES", "cs,sk"),
		"maxResults", cfg.search.MaxResults, "concurrency", cfg.search.Concurrency)
	if err := http.ListenAndServe(":"+cfg.port, mux); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writing response failed", "err", err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}
