package apienv

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteSuccess(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	WriteSuccess(recorder, http.StatusCreated, "viewer_session", map[string]string{"id": "vs_1"})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["viewer_session"]["id"] != "vs_1" {
		t.Fatalf("body = %v", body)
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	WriteError(recorder, NewError(http.StatusUnauthorized, CodeUnauthorized, "Unauthorized", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body map[string]Error
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["error"].Code != CodeUnauthorized {
		t.Fatalf("body = %v", body)
	}
}
