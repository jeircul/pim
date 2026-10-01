package azure

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type staticCredential struct{}

func (staticCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "token"}, nil
}

// newTestClient returns a Client whose every connection, whatever the URL host,
// lands on a TLS test server, so allowed ARM URLs reach it and refused ones
// would be counted if the guard let them through.
func newTestClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	tr := srv.Client().Transport.(*http.Transport).Clone()
	addr := srv.Listener.Addr().String()
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
	tr.TLSClientConfig.ServerName = "example.com"
	return &Client{cred: staticCredential{}, httpClient: newHTTPClient(tr)}
}

func TestRedirectToForeignHostIsRefused(t *testing.T) {
	for _, target := range []string{
		"https://sub.management.azure.com/steal",
		"https://evil.example/steal",
	} {
		t.Run(target, func(t *testing.T) {
			var hits atomic.Int32
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				http.Redirect(w, r, target, http.StatusFound)
			}))
			_, err := c.doRequest(t.Context(), http.MethodGet, armEndpoint+"/start", "token", nil)
			if err == nil || !strings.Contains(err.Error(), "refuse request") {
				t.Fatalf("doRequest err = %v; want refused redirect", err)
			}
			if n := hits.Load(); n != 1 {
				t.Fatalf("server saw %d requests; want 1", n)
			}
		})
	}
}

func TestDoRequestResendsBodyOnRetry(t *testing.T) {
	var mu sync.Mutex
	var bodies []string

	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	payload := []byte(`{"justification":"incident response"}`)

	resp, err := c.doRequest(context.Background(), http.MethodPut, armEndpoint+"/", "token", payload)
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

	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lengths = append(lengths, r.ContentLength)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	resp, err := c.doRequest(context.Background(), http.MethodGet, armEndpoint+"/", "token", nil)
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

func TestCheckRequestURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{"arm", armEndpoint + "/subscriptions/x/providers/Microsoft.Authorization/roleAssignmentSchedules?api-version=1", ""},
		{"graph", graphEndpoint + "/me", ""},
		{"userinfo via scope", armEndpoint + "@evil.example/resourceGroups/x", "evil.example"},
		{"plain http", "http://management.azure.com/subscriptions/x", "not https"},
		{"other host", "https://evil.example/subscriptions/x", "evil.example"},
		{"lookalike host", "https://management.azure.com.evil.example/x", "management.azure.com.evil.example"},
		{"explicit port", "https://management.azure.com:8443/x", "management.azure.com:8443"},
		{"malicious nextLink", "https://evil.example/providers/Microsoft.Authorization/roleEligibilitySchedules?$skiptoken=x", "evil.example"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkRequestURL(tt.url)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("checkRequestURL(%q) = %v; want nil", tt.url, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("checkRequestURL(%q) = %v; want error containing %q", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestPaginationRefusesForeignNextLink(t *testing.T) {
	for _, next := range []string{
		"https://evil.example/next",
		"https://management.azure.com@evil.example/next",
		"http://management.azure.com/next",
	} {
		t.Run(next, func(t *testing.T) {
			var hits atomic.Int32
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				_, _ = io.WriteString(w, `{"value":[],"nextLink":"`+next+`"}`)
			}))
			_, err := c.GetActiveAssignments(t.Context())
			if err == nil || !strings.Contains(err.Error(), "refuse request") {
				t.Fatalf("GetActiveAssignments err = %v; want refused request", err)
			}
			if n := hits.Load(); n != 1 {
				t.Fatalf("server saw %d requests; want 1", n)
			}
		})
	}
}

func TestActivateRoleRejectsCraftedScope(t *testing.T) {
	var hits atomic.Int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	_, err := c.ActivateRole(t.Context(), Role{Scope: "/subscriptions/x"}, "p", "j", 60, "@evil.example/resourceGroups/x")
	if err == nil || !strings.Contains(err.Error(), "invalid scope") {
		t.Fatalf("ActivateRole err = %v; want invalid scope", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server saw %d requests; want 0", n)
	}
}
