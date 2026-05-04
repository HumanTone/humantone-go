# Changelog

All notable changes to `github.com/humantone/humantone-go` are documented in
this file. The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.1] — 2026-05-03

Initial release.

### Added
- `Client.Humanize` for `POST /v1/humanize`, including `LevelStandard`,
  `LevelAdvanced`, and `LevelExtreme` humanization levels and `FormatText`,
  `FormatHTML`, `FormatMarkdown` output formats. SDK defaults the output
  format to `FormatText` even though the API default is HTML.
- `Client.Detect` for `POST /v1/detect`, returning the AI Likelihood Indicator.
- `Client.Account.Get` for `GET /v1/account`, returning plan, credits, and
  subscription state.
- Sentinel errors (`ErrAuthentication`, `ErrPermission`, `ErrRateLimit`,
  `ErrInsufficientCredits`, `ErrDailyLimitExceeded`, `ErrInvalidRequest`,
  `ErrNotFound`, `ErrAPIError`, `ErrTimeout`, `ErrNetwork`,
  `ErrInvalidAPIKey`) usable with `errors.Is`, plus a typed `*Error` struct
  carrying request ID, HTTP status, retry hints, and structured details
  accessible via `errors.As`.
- Eager API key validation in `NewClient` against the `^ht_[0-9a-f]{64}$`
  format, with `HUMANTONE_API_KEY` and `HUMANTONE_BASE_URL` environment
  variable fallbacks.
- Configurable timeout (default 120s), max retries (default 2), `RetryOnPost`
  opt-in, custom `*http.Client` injection, and User-Agent suffix.
- Retry policy honoring `Retry-After` (numeric and HTTP-date), exponential
  backoff with jitter, and `context.Context` cancellation throughout.
- Forward-compatible parsing of both the current v1 error shape (string
  `body.error`) and the planned v2 shape (`body.error.code`/`message`/`details`).
- Strict response validation: missing required fields, wrong types, unknown
  enum values, and unparseable timestamps surface as coercion failures
  rather than silent zero values.
- Examples for all three endpoints and a full unit test suite (≥90% coverage)
  with race detector. Integration tests gated by `integration` build tag and
  a real API key.
