package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maxRequestBodyBytes = 1 << 20

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("invalid JSON body: expected a single JSON object")
	}

	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if payload == nil {
		return
	}

	_ = json.NewEncoder(w).Encode(payload)
}
