package apienv

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllCodesContainsEveryPublicBusinessCode(t *testing.T) {
	t.Parallel()

	seen := make(map[Code]bool, len(AllCodes))
	for _, code := range AllCodes {
		if code == "" {
			t.Fatal("AllCodes contains an empty error code")
		}
		if seen[code] {
			t.Fatalf("AllCodes contains duplicate code %q", code)
		}
		seen[code] = true
	}

	want := []Code{
		CodePVCAlreadyExists,
		CodePVCNotFound,
		CodePVCAccessDenied,
		CodePVCInUse,
		CodePVCCreateForbidden,
		CodePVCDeleteForbidden,
		CodePVCExpandForbidden,
		CodePVCExpandUnsupported,
		CodePVCExpandNotIncreased,
		CodeStorageClassNotFound,
		CodeStorageClassNotVisible,
		CodeStorageClassYAMLInvalid,
		CodeStorageClassConflict,
		CodeAdminAccessDenied,
		CodeUnsupportedAccessMode,
		CodePVCMountConflict,
		CodePVCMountPending,
		CodeViewerPodCreating,
		CodeViewerPodFailed,
		CodePodSessionNotFound,
		CodeViewerSessionNotFound,
		CodeViewerSessionExpired,
		CodeAuthRequestExpired,
		CodeAuthRequestUsed,
		CodeFileBrowserLoginFailed,
		CodeHookVerifyFailed,
		CodeUnauthorized,
		CodeValidationError,
		CodeInternal,
	}
	if len(seen) != len(want) {
		t.Fatalf("AllCodes count = %d, want %d", len(seen), len(want))
	}
	for _, code := range want {
		if !IsCode(code) {
			t.Fatalf("AllCodes missing %q", code)
		}
	}
	if IsCode("NOT_A_REAL_CODE") {
		t.Fatal("IsCode accepted an unknown code")
	}
}

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
