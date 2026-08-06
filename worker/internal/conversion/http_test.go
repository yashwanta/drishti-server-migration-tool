package conversion

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPConversionDefaultsToDisabled(t *testing.T) {
	service, err := New(testWorkspace(t), 1, &fakeExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewHandler(service, false).Register(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversions", strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
}
