// Package appwrite provides a minimal HTTP client for the Appwrite Server
// REST API. It is intentionally narrow: it knows how to authenticate, page,
// retry transient failures, and decode Appwrite's error envelope. It does
// not attempt to be a full SDK.
//
// Endpoints and request/response shapes implemented here are verified
// against the Appwrite server source (github.com/appwrite/appwrite,
// tag 2.3.0) rather than guessed:
//   - GET /v1/health          -> src/Appwrite/Platform/Modules/Health/Http/Health/Get.php
//   - GET /v1/health/version  -> .../Health/Http/Health/Version/Get.php
//   - Auth headers X-Appwrite-Project / X-Appwrite-Key -> https://appwrite.io/docs/apis/rest
//   - Query envelope {"method":...,"values":[...]}     -> utopia-php/database 7.3.11 Query::toArray
package appwrite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/config"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/errs"
)

const (
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 3
	defaultBackoff    = 500 * time.Millisecond
)

// Client is a bounded, retrying HTTP client for one Appwrite endpoint.
type Client struct {
	env        config.Environment
	http       *http.Client
	maxRetries int
	backoff    time.Duration
	userAgent  string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the underlying *http.Client (e.g. for tests).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

// WithMaxRetries overrides the number of retry attempts for transient errors.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// WithBackoff overrides the base retry backoff duration (default 500ms,
// exponential from there — see backoffDelay). Exported primarily so
// tests outside this package (e.g. the fault-injection lab) can exercise
// real retry behavior without paying for real-world backoff delays.
func WithBackoff(d time.Duration) Option {
	return func(c *Client) { c.backoff = d }
}

// WithTimeout overrides the per-request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.http.Timeout = d }
}

// New constructs a Client for env. env is not validated here; call
// env.Validate() before use if credentials are required for the operation.
func New(env config.Environment, opts ...Option) *Client {
	c := &Client{
		env:        env,
		http:       &http.Client{Timeout: defaultTimeout},
		maxRetries: defaultMaxRetries,
		backoff:    defaultBackoff,
		userAgent:  "appwrite-migration-guard/amg",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Endpoint returns the configured base endpoint (no trailing slash).
func (c *Client) Endpoint() string { return c.env.Endpoint }

// apiError mirrors Appwrite's JSON error envelope, e.g.
// {"message":"...","code":401,"type":"user_unauthorized","version":"..."}
// verified against app/config/errors.php (fields: name/description/code,
// serialized by the framework as type/message/code).
type apiError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
	Type    string `json:"type"`
	Version string `json:"version"`
}

// request performs a single logical request to path with the given query
// parameters, retrying transient failures with exponential backoff. body,
// if non-nil, is marshaled as JSON. The decoded JSON response body is
// written into out (if non-nil).
func (c *Client) request(ctx context.Context, op, method, path string, query url.Values, body any, out any) error {
	return c.requestAuth(ctx, op, method, path, query, body, out, true)
}

// requestAuth is request with control over whether the API key header is
// sent. This matters for endpoints labeled scope "public" in Appwrite's
// source (e.g. GET /health/version): sending an API key on such a request
// is not simply ignored — verified against a live Appwrite Cloud project,
// attaching X-Appwrite-Key makes Appwrite evaluate the request under the
// key's "applications" role and reject it for lacking a literal "public"
// scope, which no API key can ever be granted. Public endpoints must
// therefore be called with no key at all.
func (c *Client) requestAuth(ctx context.Context, op, method, path string, query url.Values, body any, out any, sendKey bool) error {
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return errs.New(errs.KindInvalidResponse, op, fmt.Errorf("encode request body: %w", err))
		}
		bodyBytes = b
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := backoffDelay(c.backoff, attempt)
			select {
			case <-ctx.Done():
				return errs.New(errs.KindTimeout, op, ctx.Err())
			case <-time.After(delay):
			}
		}

		err := c.doOnce(ctx, op, method, path, query, bodyBytes, out, sendKey)
		if err == nil {
			return nil
		}
		lastErr = err
		if !errs.IsRetryable(err) || attempt == c.maxRetries {
			return err
		}
	}
	return lastErr
}

func (c *Client) doOnce(ctx context.Context, op, method, path string, query url.Values, bodyBytes []byte, out any, sendKey bool) error {
	u := strings.TrimRight(c.env.Endpoint, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reader io.Reader
	if bodyBytes != nil {
		reader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return errs.New(errs.KindConfiguration, op, fmt.Errorf("build request: %w", err))
	}
	req.Header.Set("X-Appwrite-Project", c.env.ProjectID)
	if sendKey && c.env.APIKey != "" {
		req.Header.Set("X-Appwrite-Key", c.env.APIKey)
	}
	req.Header.Set("User-Agent", c.userAgent)
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return errs.New(errs.KindTimeout, op, ctx.Err()).WithRetryable(false)
		}
		// The API key is only ever sent as a header, never embedded in the
		// URL or in Go's transport error text, so no redaction is needed here.
		return errs.New(errs.KindConnectivity, op, err).WithRetryable(true)
	}
	defer resp.Body.Close()

	payload, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		if ctx.Err() != nil {
			return errs.New(errs.KindTimeout, op, ctx.Err()).WithRetryable(false)
		}
		return errs.New(errs.KindInvalidResponse, op, fmt.Errorf("read response body: %w", readErr))
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out != nil && len(payload) > 0 {
			if err := json.Unmarshal(payload, out); err != nil {
				return errs.New(errs.KindInvalidResponse, op, fmt.Errorf("decode response: %w", err))
			}
		}
		return nil
	}

	return classifyHTTPError(op, resp, payload)
}

// classifyHTTPError maps a non-2xx Appwrite response to a typed *errs.Error.
// Retry eligibility follows section 24 of the project spec: retry transient
// network/server/rate-limit conditions, never retry auth/validation
// failures.
func classifyHTTPError(op string, resp *http.Response, payload []byte) error {
	var apiErr apiError
	_ = json.Unmarshal(payload, &apiErr) // best-effort; fall back to status text
	msg := apiErr.Message
	if msg == "" {
		msg = resp.Status
	}

	base := fmt.Errorf("%s", msg)

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return errs.New(errs.KindAuthentication, op, base).WithStatus(resp.StatusCode).WithRetryable(false)
	case http.StatusForbidden:
		return errs.New(errs.KindAuthorization, op, base).WithStatus(resp.StatusCode).WithRetryable(false)
	case http.StatusNotFound:
		return errs.New(errs.KindNotFound, op, base).WithStatus(resp.StatusCode).WithRetryable(false)
	case http.StatusTooManyRequests:
		return errs.New(errs.KindRateLimit, op, withRetryAfter(base, resp)).WithStatus(resp.StatusCode).WithRetryable(true)
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return errs.New(errs.KindValidation, op, base).WithStatus(resp.StatusCode).WithRetryable(false)
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return errs.New(errs.KindTimeout, op, base).WithStatus(resp.StatusCode).WithRetryable(true)
	default:
		if resp.StatusCode >= 500 {
			return errs.New(errs.KindServer, op, base).WithStatus(resp.StatusCode).WithRetryable(true)
		}
		return errs.New(errs.KindInvalidResponse, op, base).WithStatus(resp.StatusCode).WithRetryable(false)
	}
}

func withRetryAfter(base error, resp *http.Response) error {
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		return fmt.Errorf("%w (retry-after: %s)", base, ra)
	}
	return base
}

// backoffDelay returns an exponential backoff with jitter for attempt
// (1-indexed retry number).
func backoffDelay(base time.Duration, attempt int) time.Duration {
	d := base * time.Duration(1<<uint(attempt-1))
	jitter := time.Duration(rand.Int63n(int64(base)))
	return d + jitter
}

// HealthStatus is the decoded response of GET /v1/health.
type HealthStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Ping   int    `json:"ping"`
}

// Health calls GET /v1/health, which requires the health.read scope on the
// API key. Use this to confirm the API key itself is valid and authorized,
// as distinct from Version which only confirms the server is reachable.
func (c *Client) Health(ctx context.Context) (HealthStatus, error) {
	var out HealthStatus
	err := c.request(ctx, "appwrite.Health", http.MethodGet, "/health", nil, nil, &out)
	return out, err
}

// VersionInfo is the decoded response of GET /v1/health/version.
type VersionInfo struct {
	Version string `json:"version"`
}

// Version calls GET /v1/health/version, a public endpoint that confirms
// the target is an Appwrite server and reports its version. It is called
// without the API key (see requestAuth's doc comment for why) — an
// invalid or absent key must not affect this reachability check.
func (c *Client) Version(ctx context.Context) (VersionInfo, error) {
	var out VersionInfo
	err := c.requestAuth(ctx, "appwrite.Version", http.MethodGet, "/health/version", nil, nil, &out, false)
	return out, err
}
