package controller

import (
	"testing"
	"time"

	databasev1alpha1 "github.com/sudhir-u/my-mysql-operator/api/v1alpha1"
)

func TestIsChannel(t *testing.T) {
	cases := map[string]bool{
		"8.4":    true,
		"8.0":    true,
		"8.4.13": false,
		"latest": false,
		"8":      false,
		"":       false,
	}
	for in, want := range cases {
		if got := IsChannel(in); got != want {
			t.Fatalf("IsChannel(%q)=%v want %v", in, got, want)
		}
	}
}

func TestResolveImageTag(t *testing.T) {
	pins := map[string]string{"8.4": "8.4.13", "8.0": "8.0.45"}
	if got := ResolveImageTag("8.4", pins); got != "8.4.13" {
		t.Fatalf("channel resolve: got %q", got)
	}
	if got := ResolveImageTag("8.4.13", pins); got != "8.4.13" {
		t.Fatalf("explicit pin: got %q", got)
	}
	if got := ResolveImageTag("5.7", pins); got != "5.7" {
		t.Fatalf("unknown channel: got %q", got)
	}
}

func TestIsPinDrivenUpgrade(t *testing.T) {
	pins := map[string]string{"8.4": "8.4.13"}
	if !IsPinDrivenUpgrade("8.4", pins, "8.4.13") {
		t.Fatal("expected pin-driven")
	}
	if IsPinDrivenUpgrade("8.4.13", pins, "8.4.13") {
		t.Fatal("explicit Spec pin should not be pin-driven")
	}
}

func TestInMaintenanceWindow(t *testing.T) {
	w := &databasev1alpha1.MaintenanceWindowSpec{Start: "02:00", End: "04:00"}
	inside := time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC)
	outside := time.Date(2026, 11, 8, 5, 0, 0, 0, time.UTC)
	if !InMaintenanceWindow(inside, w) {
		t.Fatal("expected inside window")
	}
	if InMaintenanceWindow(outside, w) {
		t.Fatal("expected outside window")
	}
	if InMaintenanceWindow(inside, nil) {
		t.Fatal("nil window is never open")
	}
}

func TestInMaintenanceWindowCrossMidnight(t *testing.T) {
	w := &databasev1alpha1.MaintenanceWindowSpec{Start: "23:00", End: "01:00"}
	late := time.Date(2026, 11, 8, 23, 30, 0, 0, time.UTC)
	early := time.Date(2026, 11, 8, 0, 30, 0, 0, time.UTC)
	mid := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	if !InMaintenanceWindow(late, w) || !InMaintenanceWindow(early, w) {
		t.Fatal("expected cross-midnight inside")
	}
	if InMaintenanceWindow(mid, w) {
		t.Fatal("expected midday outside")
	}
}

func TestTimeUntilMaintenanceWindow(t *testing.T) {
	w := &databasev1alpha1.MaintenanceWindowSpec{Start: "02:00", End: "04:00"}
	now := time.Date(2026, 11, 8, 1, 0, 0, 0, time.UTC)
	d := TimeUntilMaintenanceWindow(now, w)
	if d != time.Hour {
		t.Fatalf("want 1h, got %v", d)
	}
}
