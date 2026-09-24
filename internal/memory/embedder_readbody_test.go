package memory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Both HTTP embedders read the response with `respBody, _ := io.ReadAll(...)`
// and then json.Unmarshal it. A read failure — a connection cut mid-body, which
// is exactly what a flaky local Ollama or a proxy timeout produces — therefore
// surfaced as "decode response (status 200): unexpected end of JSON input",
// blaming the provider's JSON for a transport fault and sending the reader to
// the wrong place. The error must name the read.

// truncatingServer promises more bytes than it sends, then hangs up, so the
// client's io.ReadAll fails with an unexpected EOF.
func truncatingServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "4096") // far more than we will write
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
		// Returning without writing Content-Length bytes truncates the body.
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOpenAIEmbedder_TruncatedBodyReportsReadFailure(t *testing.T) {
	srv := truncatingServer(t, `{"data":[{"embedding":[0.1,`)
	e := &OpenAIEmbedder{Endpoint: srv.URL, Model: "m", APIKey: "sk-test-not-a-real-key"}

	_, err := e.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("want an error for a truncated response body")
	}
	if !strings.Contains(err.Error(), "read response body") {
		t.Errorf("error should name the read failure, got %q", err)
	}
	if strings.Contains(err.Error(), "decode response") {
		t.Errorf("error blames decoding for a transport fault: %q", err)
	}
	// The auth boundary: an embedder error must never carry the API key.
	if strings.Contains(err.Error(), "sk-test-not-a-real-key") {
		t.Errorf("error leaked the API key: %q", err)
	}
}

func TestOllamaEmbedder_TruncatedBodyReportsReadFailure(t *testing.T) {
	srv := truncatingServer(t, `{"embedding":[0.1,`)
	e := &OllamaEmbedder{Endpoint: srv.URL, Model: "m"}

	_, err := e.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("want an error for a truncated response body")
	}
	if !strings.Contains(err.Error(), "read response body") {
		t.Errorf("error should name the read failure, got %q", err)
	}
	if strings.Contains(err.Error(), "decode response") {
		t.Errorf("error blames decoding for a transport fault: %q", err)
	}
}

// TestEmbedders_MalformedJSONStillReportsDecode is the counterpart: a body that
// arrives intact but is not valid JSON must still be reported as a decode
// failure, so the new read check has not simply relabelled every error.
func TestEmbedders_MalformedJSONStillReportsDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json at all`))
	}))
	t.Cleanup(srv.Close)

	if _, err := (&OpenAIEmbedder{Endpoint: srv.URL, Model: "m"}).Embed(context.Background(), "x"); err == nil {
		t.Error("openai: want a decode error")
	} else if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("openai: want a decode error, got %q", err)
	}

	if _, err := (&OllamaEmbedder{Endpoint: srv.URL, Model: "m"}).Embed(context.Background(), "x"); err == nil {
		t.Error("ollama: want a decode error")
	} else if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("ollama: want a decode error, got %q", err)
	}
}
