package gamification_test

import (
	"testing"
	"github.com/saidakmal/habbit-tracker-bot/internal/gamification"
)

func TestXPForCompletion_baseOnly(t *testing.T) {
	if xp := gamification.XPForCompletion(0); xp != 10 {
		t.Fatalf("expected 10, got %d", xp)
	}
}
func TestXPForCompletion_withStreak(t *testing.T) {
	if xp := gamification.XPForCompletion(15); xp != 25 {
		t.Fatalf("expected 25, got %d", xp)
	}
}
func TestXPForCompletion_streakCap(t *testing.T) {
	if xp := gamification.XPForCompletion(100); xp != 30 {
		t.Fatalf("expected 30, got %d", xp)
	}
}
func TestLevelFor(t *testing.T) {
	cases := []struct{ xp, want int }{{0, 1}, {99, 1}, {100, 2}, {250, 3}, {500, 4}, {1000, 5}, {1500, 6}, {2000, 7}}
	for _, c := range cases {
		if got := gamification.LevelFor(c.xp); got != c.want {
			t.Errorf("LevelFor(%d) = %d, want %d", c.xp, got, c.want)
		}
	}
}
func TestAchievementNames(t *testing.T) {
	for _, code := range gamification.AllCodes() {
		if gamification.DisplayName(code, "en") == "" {
			t.Errorf("achievement %q has no EN display name", code)
		}
	}
}
