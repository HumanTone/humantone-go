# humantone-go

Official Go client for the [HumanTone](https://humantone.io) API. Rewrites AI-generated text into natural-sounding prose. Adds an AI Likelihood Indicator that scores how AI-like any text reads.

[![Go Reference](https://pkg.go.dev/badge/github.com/humantone/humantone-go.svg)](https://pkg.go.dev/github.com/humantone/humantone-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/humantone/humantone-go)](https://goreportcard.com/report/github.com/humantone/humantone-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Install

```bash
go get github.com/humantone/humantone-go@latest
```

Requires Go 1.22 or later. Stdlib only, zero external dependencies.

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/humantone/humantone-go"
)

func main() {
    client, err := humantone.NewClient(humantone.Config{
        APIKey: os.Getenv("HUMANTONE_API_KEY"),
    })
    if err != nil {
        log.Fatal(err)
    }

    result, err := client.Humanize(context.Background(), humantone.HumanizeRequest{
        Text: "Your AI-generated draft goes here. At least 30 words.",
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Text)
    fmt.Printf("Credits used: %d\n", result.CreditsUsed)
}
```

The client also picks up `HUMANTONE_API_KEY` from the environment when `Config.APIKey` is empty:

```go
client, _ := humantone.NewClient(humantone.Config{})
```

## API

### `client.Humanize(ctx, req)`

| Field | Type | Default | Notes |
|---|---|---|---|
| `Text` | `string` | required | Min 30 words. Max depends on plan (Basic 750, Standard 1000, Pro 1500). |
| `Level` | `HumanizationLevel` | `LevelStandard` | `LevelAdvanced` and `LevelExtreme` are English-only. |
| `OutputFormat` | `OutputFormat` | `FormatText` | SDK default is `FormatText` even though the API default is `html`. |
| `CustomInstructions` | `string` | `""` | Free-form rewrite guidance. Max 1000 chars. |

Returns `*HumanizeResult` with `Text`, `OutputFormat`, `CreditsUsed`, `RequestID`.

### `client.Detect(ctx, text)`

Returns AI likelihood score 0-100. Free, but limited to 30 calls per day per account.

```go
result, err := client.Detect(ctx, "Some text...")
fmt.Println(result.AIScore)
```

### `client.Account.Get(ctx)`

```go
info, err := client.Account.Get(ctx)
fmt.Println(info.Plan.Name)
fmt.Println(info.Credits.Total)
fmt.Println(info.Plan.MaxWords)
if info.Subscription.ExpiresAt != nil {
    fmt.Println(info.Subscription.ExpiresAt.Format(time.RFC3339))
}
```

## Error handling

All SDK errors are `*humantone.Error` wrapping a sentinel.

```go
import "errors"

result, err := client.Humanize(ctx, req)
if err != nil {
    switch {
    case errors.Is(err, humantone.ErrInsufficientCredits):
        fmt.Println("Out of credits")
    case errors.Is(err, humantone.ErrRateLimit):
        var apiErr *humantone.Error
        if errors.As(err, &apiErr) {
            fmt.Printf("Wait %ds\n", apiErr.RetryAfterSeconds)
        }
    case errors.Is(err, humantone.ErrAuthentication):
        fmt.Println("Bad API key")
    default:
        var apiErr *humantone.Error
        if errors.As(err, &apiErr) {
            fmt.Printf("HumanTone error (%s): %s\n", apiErr.Code, apiErr.Message)
        } else {
            fmt.Printf("Unknown error: %v\n", err)
        }
    }
}
```

Sentinels:

- `ErrAuthentication`, `ErrPermission`, `ErrRateLimit`
- `ErrInsufficientCredits`, `ErrDailyLimitExceeded`
- `ErrInvalidRequest`, `ErrNotFound`
- `ErrAPIError`, `ErrTimeout`, `ErrNetwork`
- `ErrInvalidAPIKey` (for both missing and malformed)

`*Error` fields: `Code`, `StatusCode`, `Message`, `RequestID`, `Details`, `Retryable`, `RetryAfterSeconds`, `TimeToNextRenew`.

## Configuration

```go
client, _ := humantone.NewClient(humantone.Config{
    APIKey:      "ht_...",                    // or HUMANTONE_API_KEY env
    BaseURL:     "https://api.humantone.io",  // or HUMANTONE_BASE_URL env, default
    Timeout:     120 * time.Second,
    MaxRetries:  2,
    RetryOnPost: false,                        // POST endpoints retry only when explicit
    UserAgent:   "my-app/1.0",                 // appended to default UA after a single space
})
```

## Retry behavior

The SDK retries `Account.Get` on network errors, 5xx, and 429 (up to 2 retries). POST methods (`Humanize`, `Detect`) do **not** retry on network or 5xx by default. Humanize debits credits, so a retried request risks double-billing. Set `Config.RetryOnPost: true` to opt in. 429 always retries on every method.

`Retry-After` headers are honored in both numeric (seconds) and HTTP-date formats.

## Cancellation

All methods accept `context.Context`. Use `context.WithTimeout` or `context.WithCancel` to abort in-flight requests:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
result, err := client.Humanize(ctx, req)
```

## Limits to remember

- **Per-request word limit.** Basic 750, Standard 1000, Pro 1500. Inputs must be at least 30 words.
- **Credits.** Humanize consumes 1 credit per 100 words. Account checks and AI likelihood checks do not consume credits.
- **AI likelihood quota.** 30 checks per day per account, shared between the HumanTone web app and any API or SDK usage. Resets at midnight UTC.
- **API access.** Included on all paid plans. Free trial accounts cannot use the API.

## Get an API key

Sign up at [humantone.io](https://humantone.io). The HumanTone API is paid only.

## License

MIT

## Links

- API docs: https://humantone.io/docs/api/
- pkg.go.dev: https://pkg.go.dev/github.com/humantone/humantone-go
- Issues: https://github.com/humantone/humantone-go/issues
- Support: help@humantone.io
