package ac

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func makeResponse(code int) *http.Response {
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

func TestRetryTransport(t *testing.T) {
	t.Run("5xx retried until success", func(t *testing.T) {
		attempt := 0
		tr := &retryTransport{
			base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempt++
				if attempt == 1 {
					return makeResponse(500), nil
				}
				return makeResponse(200), nil
			}),
			maxRetries:  1,
			exemptCodes: map[int]struct{}{400: {}, 401: {}, 403: {}, 404: {}, 422: {}},
		}
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("status: got %d, want 200", resp.StatusCode)
		}
		if attempt != 2 {
			t.Errorf("attempts: got %d, want 2", attempt)
		}
	})

	t.Run("exempt 404 not retried", func(t *testing.T) {
		attempt := 0
		tr := &retryTransport{
			base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempt++
				return makeResponse(404), nil
			}),
			maxRetries:  3,
			exemptCodes: map[int]struct{}{400: {}, 401: {}, 403: {}, 404: {}, 422: {}},
		}
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != 404 {
			t.Errorf("status: got %d, want 404", resp.StatusCode)
		}
		if attempt != 1 {
			t.Errorf("attempts: got %d, want 1 (no retry for exempt code)", attempt)
		}
	})

	t.Run("exhausts retries on persistent 5xx", func(t *testing.T) {
		attempt := 0
		tr := &retryTransport{
			base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempt++
				return makeResponse(503), nil
			}),
			maxRetries:  1,
			exemptCodes: map[int]struct{}{400: {}, 401: {}, 403: {}, 404: {}, 422: {}},
		}
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != 503 {
			t.Errorf("status: got %d, want 503", resp.StatusCode)
		}
		// initial attempt + maxRetries retries
		if attempt != 2 {
			t.Errorf("attempts: got %d, want 2", attempt)
		}
	})

	t.Run("network error without body is retried", func(t *testing.T) {
		attempt := 0
		tr := &retryTransport{
			base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempt++
				if attempt == 1 {
					return nil, &networkError{"transient"}
				}
				return makeResponse(200), nil
			}),
			maxRetries:  1,
			exemptCodes: map[int]struct{}{400: {}, 401: {}, 403: {}, 404: {}, 422: {}},
		}
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("status: got %d, want 200", resp.StatusCode)
		}
		if attempt != 2 {
			t.Errorf("attempts: got %d, want 2", attempt)
		}
	})
}

type networkError struct{ msg string }

func (e *networkError) Error() string   { return e.msg }
func (e *networkError) Timeout() bool   { return false }
func (e *networkError) Temporary() bool { return true }
