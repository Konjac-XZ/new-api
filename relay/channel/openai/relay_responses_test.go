package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func TestOaiResponsesHandlerNonStreamMonitorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	responseBody := `{"id":"resp_1","object":"response","model":"gpt-4.1-mini","usage":{"input_tokens":12,"output_tokens":34,"total_tokens":46}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(responseBody)),
	}
	info := &relaycommon.RelayInfo{
		MonitorResponseBody: &strings.Builder{},
	}

	usage, newAPIErr := OaiResponsesHandler(c, info, resp)
	if newAPIErr != nil {
		t.Fatalf("unexpected newAPIErr: %v", newAPIErr)
	}
	if usage == nil {
		t.Fatal("usage is nil")
	}

	if got, want := info.MonitorResponseBody.String(), responseBody; got != want {
		t.Fatalf("monitor response body mismatch, got %q, want %q", got, want)
	}
	if got, want := recorder.Body.String(), responseBody; got != want {
		t.Fatalf("downstream response body mismatch, got %q, want %q", got, want)
	}
	if got, want := usage.PromptTokens, 12; got != want {
		t.Fatalf("prompt tokens mismatch, got %d, want %d", got, want)
	}
	if got, want := usage.CompletionTokens, 34; got != want {
		t.Fatalf("completion tokens mismatch, got %d, want %d", got, want)
	}
	if got, want := usage.TotalTokens, 46; got != want {
		t.Fatalf("total tokens mismatch, got %d, want %d", got, want)
	}
}

func TestOaiResponsesCompactionHandlerNonStreamMonitorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	responseBody := `{"id":"resp_c_1","object":"response","output":[],"usage":{"input_tokens":5,"output_tokens":7,"total_tokens":12}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(responseBody)),
	}
	info := &relaycommon.RelayInfo{
		MonitorResponseBody: &strings.Builder{},
	}

	usage, newAPIErr := OaiResponsesCompactionHandler(c, info, resp)
	if newAPIErr != nil {
		t.Fatalf("unexpected newAPIErr: %v", newAPIErr)
	}
	if usage == nil {
		t.Fatal("usage is nil")
	}
	if got, want := info.MonitorResponseBody.String(), responseBody; got != want {
		t.Fatalf("monitor response body mismatch, got %q, want %q", got, want)
	}
	if got, want := usage.PromptTokens, 5; got != want {
		t.Fatalf("prompt tokens mismatch, got %d, want %d", got, want)
	}
	if got, want := usage.CompletionTokens, 7; got != want {
		t.Fatalf("completion tokens mismatch, got %d, want %d", got, want)
	}
	if got, want := usage.TotalTokens, 12; got != want {
		t.Fatalf("total tokens mismatch, got %d, want %d", got, want)
	}
}
