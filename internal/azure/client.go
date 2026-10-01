package azure

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

const (
	apiVersion                       = "2020-10-01"
	eligibleChildResourcesAPIVersion = "2020-10-01"
	armHost                          = "management.azure.com"
	graphHost                        = "graph.microsoft.com"
	armEndpoint                      = "https://" + armHost
	graphEndpoint                    = "https://" + graphHost + "/v1.0"
	httpTimeout                      = 90 * time.Second
)

// Client handles Azure PIM operations.
type Client struct {
	cred       azcore.TokenCredential
	httpClient *http.Client
	mgSemOnce  sync.Once
	mgSem      chan struct{}
}

// mgConcurrencyLimit caps the total number of concurrent ARM
// management-group requests a single Client issues, across both the
// per-node BFS in ListAllSubscriptionsUnderMG and any concurrent expansion
// of distinct MGs by callers (e.g. pim search). The semaphore is shared
// across all such calls so nested fan-out cannot multiply concurrency
// beyond this ceiling.
const mgConcurrencyLimit = 8

// managementGroupSemaphore returns the Client's shared management-group
// request semaphore, initializing it on first use.
func (c *Client) managementGroupSemaphore() chan struct{} {
	c.mgSemOnce.Do(func() {
		c.mgSem = make(chan struct{}, mgConcurrencyLimit)
	})
	return c.mgSem
}

type childResource struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Properties struct {
		DisplayName string `json:"displayName"`
	} `json:"properties"`
}

// NewClient creates a PIM client using the best available delegated credential.
func NewClient() (*Client, error) {
	tenantID := os.Getenv("AZURE_TENANT_ID")
	var chain []azcore.TokenCredential

	if c, err := azidentity.NewAzureCLICredential(&azidentity.AzureCLICredentialOptions{TenantID: tenantID}); err == nil {
		chain = append(chain, c)
	}
	if c, err := azidentity.NewAzurePowerShellCredential(&azidentity.AzurePowerShellCredentialOptions{TenantID: tenantID}); err == nil {
		chain = append(chain, c)
	}
	if allowDeviceLogin() {
		if c, err := azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
			TenantID: tenantID,
			UserPrompt: func(_ context.Context, msg azidentity.DeviceCodeMessage) error {
				fmt.Fprintln(os.Stderr, msg.Message)
				return nil
			},
		}); err == nil {
			chain = append(chain, c)
		}
	}

	if len(chain) == 0 {
		return nil, ErrNoCredential
	}

	cred, err := azidentity.NewChainedTokenCredential(chain, nil)
	if err != nil {
		return nil, fmt.Errorf("create credential chain: %w", err)
	}

	return &Client{
		cred:       cred,
		httpClient: &http.Client{Timeout: httpTimeout},
	}, nil
}

func (c *Client) getToken(ctx context.Context, scope string) (string, error) {
	tok, err := c.cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{scope}})
	if err != nil {
		return "", fmt.Errorf("acquire token for %s: %w", scope, err)
	}
	return tok.Token, nil
}

// armToken returns a fresh ARM token. azidentity handles caching and refresh internally.
func (c *Client) armToken(ctx context.Context) (string, error) {
	return c.getToken(ctx, "https://management.azure.com/.default")
}

// graphToken returns a fresh Graph token. azidentity handles caching and refresh internally.
func (c *Client) graphToken(ctx context.Context) (string, error) {
	return c.getToken(ctx, "https://graph.microsoft.com/.default")
}

// doRequest executes an HTTP request and returns the response.
// body is wrapped in a fresh reader on every attempt: a single io.Reader would
// be drained by the first send, so 429 retries would resend an empty payload.
func (c *Client) doRequest(ctx context.Context, method, reqURL, token string, body []byte) (*http.Response, error) {
	if err := checkRequestURL(reqURL); err != nil {
		return nil, err
	}
	const maxRetries = 4
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var payload io.Reader
		if body != nil {
			payload = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, payload)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "pim/2")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("execute request: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt == maxRetries {
				return nil, errorFromResponse(resp)
			}
			resp.Body.Close()
			wait := retryAfter(resp, attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, errorFromResponse(resp)
		}
		return resp, nil
	}
	return nil, fmt.Errorf("request to %s exceeded retry limit after 429 responses", reqURL)
}

// checkRequestURL refuses any URL that would carry the bearer token to a host
// other than ARM or Graph, such as a crafted scope or a hostile nextLink.
func checkRequestURL(reqURL string) error {
	u, err := url.Parse(reqURL)
	if err != nil {
		return fmt.Errorf("parse request url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refuse request to host %q: scheme %q is not https", u.Host, u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("refuse request to host %q: url contains userinfo", u.Host)
	}
	if u.Host != armHost && u.Host != graphHost {
		return fmt.Errorf("refuse request to host %q: not an allowed Azure endpoint", u.Host)
	}
	return nil
}

// retryAfter returns the duration to wait before retrying a 429 response.
// It reads the Retry-After header (seconds) and falls back to exponential backoff.
func retryAfter(resp *http.Response, attempt int) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return time.Duration(1<<attempt) * time.Second
}

func allowDeviceLogin() bool {
	v := strings.ToLower(os.Getenv("PIM_ALLOW_DEVICE_LOGIN"))
	return v == "1" || v == "true" || v == "yes"
}
