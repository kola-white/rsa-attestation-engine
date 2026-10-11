package authority

import (
	"testing"
	"time"
)

func TestProjectGrantInterval(t *testing.T) {
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	t.Run("bounded interval converts exclusive end exactly", func(t *testing.T) {
		from, until, err := projectGrantInterval(start, &end)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !from.Equal(start) {
			t.Fatalf("from = %v, want %v", from, start)
		}
		wantUntil := end.Add(-time.Microsecond)
		if until == nil || !until.Equal(wantUntil) {
			t.Fatalf("until = %v, want %v", until, wantUntil)
		}
	})

	t.Run("unbounded interval has no end", func(t *testing.T) {
		from, until, err := projectGrantInterval(start, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !from.Equal(start) || until != nil {
			t.Fatalf("from = %v, until = %v", from, until)
		}
	})

	t.Run("zero length rejected", func(t *testing.T) {
		_, _, err := projectGrantInterval(start, &start)
		if err == nil {
			t.Fatal("expected error for zero-length interval")
		}
	})

	t.Run("reversed interval rejected", func(t *testing.T) {
		before := start.Add(-time.Microsecond)
		_, _, err := projectGrantInterval(start, &before)
		if err == nil {
			t.Fatal("expected error for reversed interval")
		}
	})

	t.Run("submicrosecond start rejected", func(t *testing.T) {
		invalid := start.Add(time.Nanosecond)
		_, _, err := projectGrantInterval(invalid, &end)
		if err == nil {
			t.Fatal("expected error for submicrosecond start")
		}
	})

	t.Run("submicrosecond end rejected", func(t *testing.T) {
		invalid := end.Add(time.Nanosecond)
		_, _, err := projectGrantInterval(start, &invalid)
		if err == nil {
			t.Fatal("expected error for submicrosecond end")
		}
	})

	t.Run("non UTC location rejected", func(t *testing.T) {
		local := start.In(time.FixedZone("PDT", -7*3600))
		_, _, err := projectGrantInterval(local, &end)
		if err == nil {
			t.Fatal("expected error for non-UTC timestamp")
		}
	})

	t.Run("zero timestamp rejected", func(t *testing.T) {
		_, _, err := projectGrantInterval(time.Time{}, &end)
		if err == nil {
			t.Fatal("expected error for missing start")
		}
	})
}
func TestProjectGrantInterval_AdjacentBoundaries(t *testing.T) {
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	transition := start.Add(time.Hour)
	end := transition.Add(time.Hour)

	_, firstUntil, err := projectGrantInterval(start, &transition)
	if err != nil {
		t.Fatalf("first interval: %v", err)
	}

	secondFrom, _, err := projectGrantInterval(transition, &end)
	if err != nil {
		t.Fatalf("second interval: %v", err)
	}

	if firstUntil == nil {
		t.Fatal("first interval unexpectedly unbounded")
	}

	if !firstUntil.Add(time.Microsecond).Equal(secondFrom) {
		t.Fatalf(
			"intervals are not exactly adjacent: first ends %v, second begins %v",
			*firstUntil,
			secondFrom,
		)
	}

	if !firstUntil.Before(secondFrom) {
		t.Fatal("projected intervals overlap")
	}
}
