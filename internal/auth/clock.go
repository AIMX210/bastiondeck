package auth

import "time"

// timeNow is indirection for tests that need deterministic TOTP validation.
var timeNow = func() time.Time { return time.Now() }
