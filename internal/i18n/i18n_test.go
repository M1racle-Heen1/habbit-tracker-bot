package i18n_test

import (
	"testing"
	"github.com/saidakmal/habbit-tracker-bot/internal/i18n"
)

func TestT_knownKey(t *testing.T) {
	got := i18n.T(i18n.RU, "error.generic")
	if got == "" || got == "error.generic" {
		t.Fatalf("expected non-empty translation, got %q", got)
	}
}
func TestT_withArgs(t *testing.T) {
	got := i18n.T(i18n.EN, "habit.created", "Running", "every 30 min", 7, 22)
	if got == "habit.created" {
		t.Fatalf("expected interpolated string, got %q", got)
	}
}
func TestT_unknownLangFallsBackToEN(t *testing.T) {
	got := i18n.T("xx", "error.generic")
	en := i18n.T(i18n.EN, "error.generic")
	if got != en {
		t.Fatalf("expected EN fallback %q, got %q", en, got)
	}
}
func TestT_missingKeyReturnsKey(t *testing.T) {
	got := i18n.T(i18n.EN, "nonexistent.key")
	if got != "nonexistent.key" {
		t.Fatalf("expected key as fallback, got %q", got)
	}
}
