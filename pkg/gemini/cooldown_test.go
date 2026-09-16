package gemini

import (
	"testing"
	"time"
)

func TestRateLimitCooldownFor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		body string
		want time.Duration
	}{
		{
			name: "no suggestion keeps the default",
			body: `{"error":{"message":"quota exceeded"}}`,
			want: DefaultRateLimitCooldown,
		},
		{
			// The real message seen from the free tier. It is under the floor,
			// so the floor applies — the point being that it is 90 seconds
			// rather than an hour.
			name: "a per-minute blip does not cost an hour",
			body: `{"error":{"message":"Quota exceeded. Please retry in 58.826548043s."}}`,
			want: MinRateLimitCooldown,
		},
		{
			name: "a suggestion above the floor is taken as given",
			body: `{"error":{"message":"Please retry in 300s"}}`,
			want: 301 * time.Second, // the parser pads by a second
		},
		{
			name: "a very short suggestion is raised to the minimum",
			body: `{"error":{"message":"Please retry in 2s"}}`,
			want: MinRateLimitCooldown,
		},
		{
			name: "a suggestion longer than the default is not shortened",
			body: `{"error":{"message":"Please retry in 120 minutes"}}`,
			want: DefaultRateLimitCooldown,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := RateLimitCooldownFor([]byte(tc.body))
			if got != tc.want {
				t.Errorf("RateLimitCooldownFor() = %v, want %v", got, tc.want)
			}
			if got > DefaultRateLimitCooldown {
				t.Errorf("cooldown %v exceeds the default ceiling %v", got, DefaultRateLimitCooldown)
			}
			if got < MinRateLimitCooldown {
				t.Errorf("cooldown %v is below the minimum %v", got, MinRateLimitCooldown)
			}
		})
	}
}
