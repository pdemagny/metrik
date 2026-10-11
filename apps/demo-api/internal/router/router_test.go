package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

func TestNewRouter(t *testing.T) {
	t.Parallel()
	r, err := NewRouter()
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	serve := func(t *testing.T, path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	t.Run("liveness", func(t *testing.T) {
		t.Parallel()
		rec := serve(t, "/v1/.well-known/live")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var body APIResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Status != http.StatusOK || body.Message != "liveness" {
			t.Errorf("body = %+v, want status 200 and message liveness", body)
		}
	})

	t.Run("openapi", func(t *testing.T) {
		t.Parallel()
		rec := serve(t, "/openapi")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		served := rec.Body.Bytes()
		file, err := os.ReadFile("../../api/openapi.json")
		if err != nil {
			t.Fatalf("read contract file: %v", err)
		}
		var got, want any
		if err := json.Unmarshal(served, &got); err != nil {
			t.Fatalf("decode served contract: %v", err)
		}
		if err := json.Unmarshal(file, &want); err != nil {
			t.Fatalf("decode contract file: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Error("served contract differs from api/openapi.json: update the file")
		}
	})
}
