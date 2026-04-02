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
func TestNewNavKeysExistAllLangs(t *testing.T) {
	keys := []string{
		"nav.today", "nav.my_habits", "nav.add_habit", "nav.stats", "nav.settings",
		"nav.menu_hint",
		"habit.created_with_defaults",
		"settings.header", "settings.lang_btn", "settings.tz_btn",
		"onboarding.skip_btn",
		"habit.pause_btn", "habit.resume_btn", "habit.done_btn", "habit.delete_btn",
		"history.btn",
	}
	for _, key := range keys {
		for _, lang := range []i18n.Lang{i18n.RU, i18n.EN, i18n.KZ} {
			got := i18n.T(lang, key)
			if got == key {
				t.Errorf("lang %s: key %q is missing", lang, key)
			}
			if got == "" {
				t.Errorf("lang %s: key %q is empty", lang, key)
			}
		}
	}
}

func TestWelcomeScreenKeyExistsAllLangs(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.RU, i18n.EN, i18n.KZ} {
		got := i18n.T(lang, "onboarding.welcome_screen")
		if got == "onboarding.welcome_screen" {
			t.Errorf("lang %s: key onboarding.welcome_screen is missing", lang)
		}
		if got == "" {
			t.Errorf("lang %s: key onboarding.welcome_screen is empty", lang)
		}
	}
}

func TestStatsKeysExistAllLangs(t *testing.T) {
	keys := []string{
		"stats.today_line",
		"stats.week_line",
		"stats.month_line",
		"stats.habit_btn",
		"stats.habit_btn_paused",
	}
	langs := []i18n.Lang{i18n.RU, i18n.EN, i18n.KZ}
	for _, lang := range langs {
		for _, key := range keys {
			got := i18n.T(lang, key)
			if got == key || got == "" {
				t.Errorf("lang=%s key=%s: missing translation (got %q)", lang, key, got)
			}
		}
	}
}
