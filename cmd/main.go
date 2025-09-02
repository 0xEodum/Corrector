package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"

	sc "corrector/internal/corrector"
	"corrector/internal/customdict"
)

func main() {
	cfg := sc.CorrectorConfig{
		MaxEditDistance:  2,
		FreqTemperature:  2.0,
		TopKSuggestions:  8,
		BetaWeight:       1.0,
		LambdaPenalty:    0.9,
		GammaMorph:       1.05,
		MarginThreshold:  0.25,
		TauInVocab:       0.5,
		TauOutVocab:      0.3,
		UseSymSpell:      true,
		UseMorphology:    true,
		EnableContext:    true,
		FilterShortWords: true,
		TransposeCost:    0.6,
		NeighborInsDel:   0.9,
		KeyboardNearSub:  0.6,
	}

	redisAddr := getenv("REDIS_ADDR", "localhost:6379")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB := getEnvInt("REDIS_DB", 0)

	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	})

	dict := customdict.New(client)

	dictionaryPath := getenv("DICTIONARY_PATH", "ru.txt")
	corrector, err := sc.NewSpellCorrector(cfg, dictionaryPath, dict)
	if err != nil {
		log.Fatalf("init error: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/correct", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}
		res := corrector.CorrectText(req.Text, false)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"original":    res.Original,
			"corrected":   res.Corrected,
			"suggestions": res.Suggestions,
		})
	})

	mux.HandleFunc("/api/v1/custom-word", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Word string `json:"word"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Word) == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}
		if err := corrector.AddCustomWord(req.Word); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/v1/custom-word/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		word := strings.TrimPrefix(r.URL.Path, "/api/v1/custom-word/")
		if word == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "word is required"})
			return
		}
		if err := corrector.RemoveCustomWord(word); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := getenv("HTTP_ADDR", ":8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func getenv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if i, err := strconv.Atoi(v); err == nil {
		return i
	}
	return def
}
