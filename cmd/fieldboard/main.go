package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/infrai-examples/field-service-live-dashboard/internal/fieldservice"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	channel := os.Getenv("INFRAI_REALTIME_CHANNEL")
	if channel == "" {
		log.Fatal("INFRAI_REALTIME_CHANNEL is required")
	}

	stream := fieldservice.NewStatusStream(fieldservice.NewClient(key, channel))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /device-updates", func(w http.ResponseWriter, r *http.Request) {
		var update fieldservice.DeviceUpdate
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&update); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device update"})
			return
		}

		result, err := stream.Apply(r.Context(), update)
		if err != nil {
			var apiErr *fieldservice.InfraiError
			if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
				writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Message, "code": apiErr.Code})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream request failed"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	server := &http.Server{Addr: ":8080", Handler: mux}
	log.Printf("fieldboard listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}
