package provider

import "errors"

// ErrRateLimited is the error a provider implementation should wrap (via
// fmt.Errorf("...: %w", ErrRateLimited)) when it detects a rate-limit
// response (e.g. HTTP 429), so callers can retry via errors.Is without
// knowing which specific provider they're talking to.
var ErrRateLimited = errors.New("provider: rate limited")
