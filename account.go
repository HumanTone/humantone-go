package humantone

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Plan describes the account's current subscription tier as returned by
// /v1/account.
type Plan struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	MaxWords       int    `json:"max_words"`
	MonthlyCredits int    `json:"monthly_credits"`
	APIAccess      bool   `json:"api_access"`
}

// Credits is the per-bucket credit breakdown returned by /v1/account.
type Credits struct {
	Trial        int `json:"trial"`
	Subscription int `json:"subscription"`
	Extra        int `json:"extra"`
	Total        int `json:"total"`
}

// Subscription describes the active subscription state. ExpiresAt is nil
// when the API omits the field or returns JSON null.
type Subscription struct {
	Active    bool       `json:"active"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// AccountInfo is the response from Client.Account.Get.
type AccountInfo struct {
	Plan         Plan         `json:"plan"`
	Credits      Credits      `json:"credits"`
	Subscription Subscription `json:"subscription"`
	RequestID    string       `json:"-"`
}

// AccountResource exposes the /v1/account operations. Obtain it from
// Client.Account; do not construct directly.
type AccountResource struct {
	client *Client
}

// accountResponseWire is the strict-coercion landing zone for the
// /v1/account success body. Pointer-typed required fields make "missing"
// detectable; a missing nested object becomes a coercion failure.
type accountResponseWire struct {
	Plan         *planWire         `json:"plan"`
	Credits      *creditsWire      `json:"credits"`
	Subscription *subscriptionWire `json:"subscription"`
	RequestID    *string           `json:"request_id"`
}

type planWire struct {
	ID             *string `json:"id"`
	Name           *string `json:"name"`
	MaxWords       *int    `json:"max_words"`
	MonthlyCredits *int    `json:"monthly_credits"`
	APIAccess      *bool   `json:"api_access"`
}

type creditsWire struct {
	Trial        *int `json:"trial"`
	Subscription *int `json:"subscription"`
	Extra        *int `json:"extra"`
	Total        *int `json:"total"`
}

type subscriptionWire struct {
	Active    *bool      `json:"active"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Get fetches the account info: plan, credits, subscription state. It does
// not consume credits.
func (a *AccountResource) Get(ctx context.Context) (*AccountInfo, error) {
	body, headers, apiErr := a.client.executeJSON(ctx, http.MethodGet, "/v1/account", nil)
	if apiErr != nil {
		return nil, apiErr
	}

	var resp accountResponseWire
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, coercionFailure("/v1/account", body, err.Error())
	}

	if resp.Plan == nil {
		return nil, coercionFailure("/v1/account", body, "plan: required object missing")
	}
	if resp.Credits == nil {
		return nil, coercionFailure("/v1/account", body, "credits: required object missing")
	}
	if resp.Subscription == nil {
		return nil, coercionFailure("/v1/account", body, "subscription: required object missing")
	}

	plan, planErr := buildPlan(body, resp.Plan)
	if planErr != nil {
		return nil, planErr
	}
	credits, creditsErr := buildCredits(body, resp.Credits)
	if creditsErr != nil {
		return nil, creditsErr
	}
	sub, subErr := buildSubscription(body, resp.Subscription)
	if subErr != nil {
		return nil, subErr
	}

	out := &AccountInfo{
		Plan:         plan,
		Credits:      credits,
		Subscription: sub,
	}
	out.RequestID = pickRequestID(resp.RequestID, headers)
	return out, nil
}

func buildPlan(body []byte, w *planWire) (Plan, *Error) {
	if w.ID == nil {
		return Plan{}, coercionFailure("/v1/account", body, "plan.id: required field missing")
	}
	if w.Name == nil {
		return Plan{}, coercionFailure("/v1/account", body, "plan.name: required field missing")
	}
	if w.MaxWords == nil {
		return Plan{}, coercionFailure("/v1/account", body, "plan.max_words: required field missing")
	}
	if w.MonthlyCredits == nil {
		return Plan{}, coercionFailure("/v1/account", body, "plan.monthly_credits: required field missing")
	}
	if w.APIAccess == nil {
		return Plan{}, coercionFailure("/v1/account", body, "plan.api_access: required field missing")
	}
	return Plan{
		ID:             *w.ID,
		Name:           *w.Name,
		MaxWords:       *w.MaxWords,
		MonthlyCredits: *w.MonthlyCredits,
		APIAccess:      *w.APIAccess,
	}, nil
}

func buildCredits(body []byte, w *creditsWire) (Credits, *Error) {
	if w.Trial == nil {
		return Credits{}, coercionFailure("/v1/account", body, "credits.trial: required field missing")
	}
	if w.Subscription == nil {
		return Credits{}, coercionFailure("/v1/account", body, "credits.subscription: required field missing")
	}
	if w.Extra == nil {
		return Credits{}, coercionFailure("/v1/account", body, "credits.extra: required field missing")
	}
	if w.Total == nil {
		return Credits{}, coercionFailure("/v1/account", body, "credits.total: required field missing")
	}
	return Credits{
		Trial:        *w.Trial,
		Subscription: *w.Subscription,
		Extra:        *w.Extra,
		Total:        *w.Total,
	}, nil
}

func buildSubscription(body []byte, w *subscriptionWire) (Subscription, *Error) {
	if w.Active == nil {
		return Subscription{}, coercionFailure("/v1/account", body, "subscription.active: required field missing")
	}
	return Subscription{
		Active:    *w.Active,
		ExpiresAt: w.ExpiresAt,
	}, nil
}
