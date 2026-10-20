package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"example.com/scheduled-job-failure-reporter/jobwatch"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	reporter := jobwatch.Reporter{Capturer: jobwatch.NewInfraiClient(apiKey)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /job-events", func(w http.ResponseWriter, req *http.Request) {
		var event jobwatch.BuildEvent
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&event); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid job event"})
			return
		}
		outcome, err := reporter.Record(req.Context(), event)
		if err != nil {
			var validationErr *jobwatch.ValidationError
			if errors.As(err, &validationErr) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": validationErr.Error()})
				return
			}
			var apiErr *jobwatch.APIError
			if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
				writeJSON(w, apiErr.Status, map[string]string{"error": apiErr.Code})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "capture transport failed"})
			return
		}
		writeJSON(w, http.StatusOK, outcome)
	})

	address := os.Getenv("LISTEN_ADDR")
	if address == "" {
		address = ":8080"
	}
	log.Printf("job failure reporter listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
