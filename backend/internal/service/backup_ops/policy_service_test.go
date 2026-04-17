package backup_ops

import "testing"

func TestNormalizePolicyValues_Defaults(t *testing.T) {
	interval, retention, timezone, drillEnabled, drillInterval, targetRef, err := normalizePolicyValues(0, 0, "", nil, 0, "")
	if err != nil {
		t.Fatalf("normalizePolicyValues returned error: %v", err)
	}
	if interval != defaultIntervalHours {
		t.Fatalf("expected default interval %d, got %d", defaultIntervalHours, interval)
	}
	if retention != defaultRetentionCount {
		t.Fatalf("expected default retention %d, got %d", defaultRetentionCount, retention)
	}
	if timezone != defaultTimezone {
		t.Fatalf("expected default timezone %s, got %s", defaultTimezone, timezone)
	}
	if !drillEnabled {
		t.Fatalf("expected default drill enabled")
	}
	if drillInterval != defaultDrillIntervalDay {
		t.Fatalf("expected default drill interval %d, got %d", defaultDrillIntervalDay, drillInterval)
	}
	if targetRef != "powerx_bak" {
		t.Fatalf("expected default target_ref powerx_bak, got %s", targetRef)
	}
}

func TestNormalizePolicyValues_InvalidTimezone(t *testing.T) {
	_, _, _, _, _, _, err := normalizePolicyValues(6, 14, "Invalid/Timezone", nil, 7, "powerx_bak")
	if err == nil {
		t.Fatalf("expected invalid timezone error")
	}
}

func TestParseScheduleHours(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"6h", 6},
		{"30m", 1},
		{"2d", 48},
		{" 12H ", 12},
		{"bad", defaultIntervalHours},
		{"", defaultIntervalHours},
	}
	for _, c := range cases {
		got := parseScheduleHours(c.in)
		if got != c.want {
			t.Fatalf("parseScheduleHours(%q): want %d, got %d", c.in, c.want, got)
		}
	}
}
