package humantone

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// Default values applied when Config fields are zero-valued.
const (
	defaultBaseURL    = "https://api.humantone.io"
	defaultTimeout    = 120 * time.Second
	defaultMaxRetries = 2
)

// apiKeyPattern is the canonical HumanTone API key format: "ht_" followed by
// exactly 64 lowercase hex characters, total 67 characters.
var apiKeyPattern = regexp.MustCompile(`^ht_[0-9a-f]{64}$`)

// Config configures a Client. The zero value is not directly usable —
// NewClient performs validation per §5 of the spec — but every field has a
// safe default applied when left at its zero value:
//
//   - APIKey: required (or set HUMANTONE_API_KEY); whitespace is trimmed.
//   - BaseURL: defaults to "https://api.humantone.io"; HUMANTONE_BASE_URL is
//     consulted if Config.BaseURL is empty/whitespace.
//   - Timeout: defaults to 120 seconds.
//   - MaxRetries: defaults to 2 (i.e., up to 3 total attempts).
//   - RetryOnPost: defaults to false. When true, POST endpoints retry on
//     transient transport, 5xx, and the reserved humanize 200+success:false
//     case. 429 retries on POST regardless of this flag.
//   - HTTPClient: optional; injected as-is. The SDK does not modify its
//     Timeout field. When nil, the SDK constructs an *http.Client with
//     Timeout = Config.Timeout.
//   - UserAgent: optional suffix appended to the SDK's User-Agent string,
//     separated by a single space. Whitespace is trimmed.
type Config struct {
	APIKey      string
	BaseURL     string
	Timeout     time.Duration
	MaxRetries  int
	RetryOnPost bool
	HTTPClient  *http.Client
	UserAgent   string
}

// Client is the entry point for all HumanTone SDK calls. It is safe for
// concurrent use by multiple goroutines once constructed.
//
// Construct one with NewClient and reuse it across requests; the underlying
// *http.Client maintains its own connection pool.
type Client struct {
	// Account exposes /v1/account operations. Always non-nil after NewClient.
	Account *AccountResource

	cfg        Config
	baseURL    string
	httpClient *http.Client
	userAgent  string
	sdkVersion string
}

// NewClient validates the Config per §5 and constructs a *Client. It returns
// a *Error wrapping ErrInvalidAPIKey when the API key is missing (Code
// ErrCodeMissingAPIKey) or malformed (Code ErrCodeInvalidAPIKey). Validation
// is eager — the returned client never makes an HTTP request without first
// having seen a syntactically valid key.
func NewClient(cfg Config) (*Client, error) {
	resolvedKey, keyErr := resolveAPIKey(cfg.APIKey)
	if keyErr != nil {
		return nil, keyErr
	}
	cfg.APIKey = resolvedKey

	cfg.BaseURL = resolveBaseURL(cfg.BaseURL)

	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaultMaxRetries
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}

	sdkVer := resolveSDKVersion()
	ua := buildUserAgent(sdkVer, sanitizedGoVersion(), cfg.UserAgent)

	c := &Client{
		cfg:        cfg,
		baseURL:    cfg.BaseURL,
		httpClient: httpClient,
		userAgent:  ua,
		sdkVersion: sdkVer,
	}
	c.Account = &AccountResource{client: c}
	return c, nil
}

// resolveAPIKey applies §5.2/§5.3: prefer Config.APIKey over the env var,
// trim both, treat trimmed-empty as unset, and validate the regex.
func resolveAPIKey(configKey string) (string, *Error) {
	cfgKey := strings.TrimSpace(configKey)
	if cfgKey != "" {
		if !apiKeyPattern.MatchString(cfgKey) {
			return "", newError(ErrInvalidAPIKey, ErrCodeInvalidAPIKey,
				"humantone: invalid API key format. Expected ht_ prefix followed by 64 hex characters.")
		}
		return cfgKey, nil
	}

	envKey := strings.TrimSpace(os.Getenv("HUMANTONE_API_KEY"))
	if envKey == "" {
		return "", newError(ErrInvalidAPIKey, ErrCodeMissingAPIKey,
			"humantone: missing API key. Pass APIKey in Config or set HUMANTONE_API_KEY environment variable. Get a key at https://app.humantone.io/settings/api")
	}
	if !apiKeyPattern.MatchString(envKey) {
		return "", newError(ErrInvalidAPIKey, ErrCodeInvalidAPIKey,
			"humantone: invalid API key format. Expected ht_ prefix followed by 64 hex characters.")
	}
	return envKey, nil
}

// resolveBaseURL applies §5.2's empty-as-unset rule to both Config.BaseURL
// and HUMANTONE_BASE_URL, falling back to the SDK default.
func resolveBaseURL(configBaseURL string) string {
	if v := strings.TrimSpace(configBaseURL); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("HUMANTONE_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultBaseURL
}

// executeJSON performs a complete request lifecycle: marshal the request
// body (if any), drive the retry loop, and return the parsed success body
// alongside the response headers (for header-based RequestID fallback).
//
// Endpoint methods are responsible for unmarshaling and validating the
// returned body into their own typed result.
func (c *Client) executeJSON(ctx context.Context, method, path string, reqBody any) ([]byte, http.Header, *Error) {
	var body []byte
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return nil, nil, newError(ErrInvalidRequest, ErrCodeInvalidRequest, err.Error())
		}
		body = b
	}

	return c.executeWithRetry(ctx, method, path, func(ctx context.Context) ([]byte, http.Header, *Error) {
		status, headers, respBody, transportErr := c.doOnce(ctx, method, path, body)
		if transportErr != nil {
			return nil, nil, transportErr
		}
		successBody, parseErr := parseResponse(method, path, status, headers, respBody)
		if parseErr != nil {
			return nil, headers, parseErr
		}
		return successBody, headers, nil
	})
}
