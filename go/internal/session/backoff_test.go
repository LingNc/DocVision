package session

import (
	"strings"
	"testing"
	"time"
)

// Backoff must never overflow into a negative wait (real runs logged
// "-2562047h47m16.854775808s" = math.MinInt64) and account-level errors
// must not be retried at all.
func TestBackoffAndBalanceDetection(t *testing.T) {
	for i := 0; i < 80; i++ {
		w := backoffWait(i, 2*time.Second, 60*time.Second)
		if w <= 0 || w > 60*time.Second {
			t.Fatalf("backoffWait(%d) = %v", i, w)
		}
	}
	if w := backoffWait(-3, 2*time.Second, 60*time.Second); w != 2*time.Second {
		t.Fatalf("negative attempt must clamp to base, got %v", w)
	}
	for _, msg := range []string{
		`http 429: {"error":{"message":"余额不足或无可用资源包,请充值。","code":"1113"}}`,
		"insufficient quota for this api key",
		"You exceeded your current quota, please check your plan and billing",
	} {
		if !insufficientBalance(strings.ToLower(msg)) {
			t.Fatalf("must be treated as an account error: %s", msg)
		}
	}
	for _, msg := range []string{"429 too many requests, retry later", "rate limit exceeded"} {
		if insufficientBalance(strings.ToLower(msg)) {
			t.Fatalf("a plain rate limit must stay retryable: %s", msg)
		}
	}
}
