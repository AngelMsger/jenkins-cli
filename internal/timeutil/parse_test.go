package timeutil

import (
	"testing"
	"time"
)

func TestFromMillis(t *testing.T) {
	got := FromMillis(1_700_000_000_000)
	want := time.UnixMilli(1_700_000_000_000).UTC()
	if !got.Equal(want) {
		t.Fatalf("FromMillis = %s, want %s", got, want)
	}
	if got.Location() != time.UTC {
		t.Fatalf("FromMillis location = %s, want UTC", got.Location())
	}
}

func TestHumanSinceAt(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"just now", now.Add(-10 * time.Second), "just now"},
		{"minutes", now.Add(-5 * time.Minute), "5m ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"days", now.Add(-50 * time.Hour), "2d ago"},
		{"future", now.Add(2 * time.Hour), "in 2h"},
		{"future moment", now.Add(5 * time.Second), "in a moment"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := humanSinceAt(c.t, now); got != c.want {
				t.Fatalf("humanSinceAt(%s) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}
