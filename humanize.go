package humantone

import (
	"context"
	"encoding/json"
	"net/http"
)

// HumanizationLevel selects the rewriting intensity for /v1/humanize.
type HumanizationLevel string

// Recognized humanization levels. LevelAdvanced and LevelExtreme are
// English-only at the API; sending them with non-English text returns an
// ErrInvalidRequest with code ErrCodeLanguageNotSupported.
const (
	LevelStandard HumanizationLevel = "standard"
	LevelAdvanced HumanizationLevel = "advanced"
	LevelExtreme  HumanizationLevel = "extreme"
)

// OutputFormat selects the response format for humanized content.
type OutputFormat string

// Recognized output formats. The SDK defaults to FormatText (overriding the
// API's own default of FormatHTML) so that callers receive plain text unless
// they explicitly opt into HTML or markdown.
const (
	FormatText     OutputFormat = "text"
	FormatHTML     OutputFormat = "html"
	FormatMarkdown OutputFormat = "markdown"
)

// HumanizeRequest is the input to Client.Humanize.
//
// Text is required (minimum 30 words, plan-dependent maximum). Level and
// OutputFormat have safe defaults applied when left at the zero value.
// CustomInstructions, when set, must be at most 1000 characters; the SDK
// does not enforce this locally — the API will reject longer values.
type HumanizeRequest struct {
	Text               string
	Level              HumanizationLevel
	OutputFormat       OutputFormat
	CustomInstructions string
}

// HumanizeResult is the output of Client.Humanize.
type HumanizeResult struct {
	Text         string       `json:"content"`
	OutputFormat OutputFormat `json:"output_format"`
	CreditsUsed  int          `json:"credits_used"`
	RequestID    string       `json:"request_id"`
}

// humanizeRequestWire is the JSON shape sent on the wire. It exists so that
// HumanizeRequest can use the idiomatic Go field name "Text" while the API
// receives "content", and so default values are applied in normal Go code
// rather than via custom MarshalJSON.
type humanizeRequestWire struct {
	Content            string `json:"content"`
	HumanizationLevel  string `json:"humanization_level"`
	OutputFormat       string `json:"output_format"`
	CustomInstructions string `json:"custom_instructions,omitempty"`
}

// humanizeResponseWire is the strict-coercion landing zone for the JSON
// success body. Pointer fields make "missing" detectable; type mismatches
// surface as json.Unmarshal errors which become coercion failures.
type humanizeResponseWire struct {
	Content      *string `json:"content"`
	OutputFormat *string `json:"output_format"`
	CreditsUsed  *int    `json:"credits_used"`
	RequestID    *string `json:"request_id"`
}

// Humanize rewrites AI-generated text into natural-sounding human prose.
// Returns a *HumanizeResult on success; on failure returns a *Error wrapping
// one of the package's sentinel errors.
func (c *Client) Humanize(ctx context.Context, req HumanizeRequest) (*HumanizeResult, error) {
	wire := humanizeRequestWire{
		Content:            req.Text,
		HumanizationLevel:  string(req.Level),
		OutputFormat:       string(req.OutputFormat),
		CustomInstructions: req.CustomInstructions,
	}
	if wire.HumanizationLevel == "" {
		wire.HumanizationLevel = string(LevelStandard)
	}
	if wire.OutputFormat == "" {
		wire.OutputFormat = string(FormatText)
	}

	body, headers, apiErr := c.executeJSON(ctx, http.MethodPost, "/v1/humanize", wire)
	if apiErr != nil {
		return nil, apiErr
	}

	var resp humanizeResponseWire
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, coercionFailure("/v1/humanize", body, err.Error())
	}

	if resp.Content == nil {
		return nil, coercionFailure("/v1/humanize", body, "content: required field missing")
	}
	if resp.OutputFormat == nil {
		return nil, coercionFailure("/v1/humanize", body, "output_format: required field missing")
	}
	if !isKnownOutputFormat(*resp.OutputFormat) {
		return nil, coercionFailure("/v1/humanize", body, "output_format: unknown enum value "+*resp.OutputFormat)
	}
	if resp.CreditsUsed == nil {
		return nil, coercionFailure("/v1/humanize", body, "credits_used: required field missing")
	}

	out := &HumanizeResult{
		Text:         *resp.Content,
		OutputFormat: OutputFormat(*resp.OutputFormat),
		CreditsUsed:  *resp.CreditsUsed,
	}
	out.RequestID = pickRequestID(resp.RequestID, headers)
	return out, nil
}

// isKnownOutputFormat reports whether s is one of the API's documented enum
// values. Used to guard against the SDK silently accepting future server-
// side additions (e.g., a new "csv" format) that the typed enum cannot
// represent.
func isKnownOutputFormat(s string) bool {
	switch OutputFormat(s) {
	case FormatText, FormatHTML, FormatMarkdown:
		return true
	}
	return false
}

// pickRequestID applies §4.14 precedence to a per-endpoint optional string
// field plus the response headers.
func pickRequestID(bodyValue *string, headers http.Header) string {
	if bodyValue != nil && *bodyValue != "" {
		return *bodyValue
	}
	if headers != nil {
		if h := headers.Get("X-Request-Id"); h != "" {
			return h
		}
	}
	return ""
}

// coercionFailure builds the *Error returned when a 2xx response body fails
// the strict typing rules of §4.8 step 8. Never retried; always preserves a
// truncated copy of the raw body for debugging.
func coercionFailure(path string, body []byte, reason string) *Error {
	e := newError(ErrAPIError, ErrCodeAPIError, "Malformed response from HumanTone API. See exception details.")
	e.StatusCode = http.StatusOK
	e.Retryable = false
	e.Details = map[string]any{
		"raw_body":       truncate(body, maxRawBodyBytes),
		"coercion_error": reason,
		"endpoint":       path,
	}
	return e
}
