package azure

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestDoRequestResendsBodyOnRetry(t *testing.T) {
	var mu sync.Mutex
	var bodies []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		mu.Lock()
		bodies = append(bodies, string(b))
		n := len(bodies)
		mu.Unlock()

		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client()}
	payload := []byte(`{"justification":"incident response"}`)

	resp, err := c.doRequest(context.Background(), http.MethodPut, srv.URL, "token", payload)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	defer resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(bodies))
	}
	for i, got := range bodies {
		if got != string(payload) {
			t.Errorf("attempt %d body = %q, want %q", i+1, got, payload)
		}
	}
}

func TestDoRequestNilBodySendsNothing(t *testing.T) {
	var mu sync.Mutex
	var lengths []int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lengths = append(lengths, r.ContentLength)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client()}
	resp, err := c.doRequest(context.Background(), http.MethodGet, srv.URL, "token", nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	defer resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(lengths) != 1 || lengths[0] != 0 {
		t.Fatalf("content lengths = %v, want [0]", lengths)
	}
}
