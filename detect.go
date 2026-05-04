package humantone

import (
	"context"
	"encoding/json"
	"net/http"
)

// DetectResult is the output of Client.Detect. AIScore is the 0-100 AI
// likelihood; higher means more AI-like. RequestID, when non-empty, echoes
// the X-Request-Id of the response (or the body's request_id field, if the
// API ever starts returning it).
type DetectResult struct {
	AIScore   int    `json:"ai_score"`
	RequestID string `json:"-"`
}

// detectRequestWire is the JSON shape sent on the wire for /v1/detect.
type detectRequestWire struct {
	Content string `json:"content"`
}

// detectResponseWire is the strict-coercion landing zone for the /v1/detect
// success body.
type detectResponseWire struct {
	AIScore   *int    `json:"ai_score"`
	RequestID *string `json:"request_id"`
}

// Detect returns the AI Likelihood Indicator score for the given text. The
// score ranges from 0 (clearly human) to 100 (clearly AI-generated).
//
// /v1/detect is free of credit cost but capped at 30 calls per day per
// account, shared across the HumanTone web app and the API. Exceeding the
// cap returns *Error wrapping ErrDailyLimitExceeded with TimeToNextRenew
// indicating the seconds until the daily counter resets.
func (c *Client) Detect(ctx context.Context, text string) (*DetectResult, error) {
	wire := detectRequestWire{Content: text}

	body, headers, apiErr := c.executeJSON(ctx, http.MethodPost, "/v1/detect", wire)
	if apiErr != nil {
		return nil, apiErr
	}

	var resp detectResponseWire
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, coercionFailure("/v1/detect", body, err.Error())
	}

	if resp.AIScore == nil {
		return nil, coercionFailure("/v1/detect", body, "ai_score: required field missing")
	}
	if *resp.AIScore < 0 || *resp.AIScore > 100 {
		return nil, coercionFailure("/v1/detect", body, "ai_score: out of range [0,100]")
	}

	out := &DetectResult{AIScore: *resp.AIScore}
	out.RequestID = pickRequestID(resp.RequestID, headers)
	return out, nil
}
