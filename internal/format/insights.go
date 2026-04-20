package format

import (
	"fmt"
	"strings"

	"github.com/saidakmal/habbit-tracker-bot/internal/domain"
	"github.com/saidakmal/habbit-tracker-bot/internal/i18n"
)

func BuildMoodSummary(moods []domain.MoodLog, lang string) string {
	counts := [4]int{} // index 1=Tough, 2=Okay, 3=Great
	for _, m := range moods {
		if m.Mood >= 1 && m.Mood <= 3 {
			counts[m.Mood]++
		}
	}
	var parts []string
	if counts[3] > 0 {
		parts = append(parts, fmt.Sprintf("😊×%d", counts[3]))
	}
	if counts[2] > 0 {
		parts = append(parts, fmt.Sprintf("😐×%d", counts[2]))
	}
	if counts[1] > 0 {
		parts = append(parts, fmt.Sprintf("😞×%d", counts[1]))
	}
	summary := strings.Join(parts, " ")
	return i18n.T(lang, "mood.week_summary", summary)
}

// BestAndWorstDay returns the day-of-week indices (0=Sun…6=Sat) with the
// highest and lowest activity counts. Returns -1 for both if the map is empty.
func BestAndWorstDay(dow map[int]int) (best, worst int) {
	if len(dow) == 0 {
		return -1, -1
	}
	best, worst = -1, -1
	for d, cnt := range dow {
		if best == -1 || cnt > dow[best] {
			best = d
		}
		if worst == -1 || cnt < dow[worst] {
			worst = d
		}
	}
	return best, worst
}

func BuildDayOfWeekInsight(dow map[int]int, best, worst int, lang string) string {
	if best < 0 || worst < 0 {
		return ""
	}
	total := 0
	for _, v := range dow {
		total += v
	}
	if total == 0 {
		return ""
	}
	bestPct := dow[best] * 100 * 7 / total
	worstPct := dow[worst] * 100 * 7 / total
	bestName := WeekdayName(best, lang)
	worstName := WeekdayName(worst, lang)
	return i18n.T(lang, "insights.tip", bestName, bestPct, worstName, worstPct)
}

func WeekdayName(dow int, lang string) string {
	return i18n.T(lang, fmt.Sprintf("weekday.%d", dow))
}

func CountMood(moods []domain.MoodLog, mood int) int {
	n := 0
	for _, m := range moods {
		if m.Mood == mood {
			n++
		}
	}
	return n
}
