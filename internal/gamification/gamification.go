package gamification

import "fmt"

const baseXP = 10
const maxStreakBonus = 20

func XPForCompletion(streak int) int {
	bonus := streak
	if bonus > maxStreakBonus {
		bonus = maxStreakBonus
	}
	return baseXP + bonus
}

var levelThresholds = []int{0, 100, 250, 500, 1000}

func LevelFor(xp int) int {
	lv := 1
	for i, t := range levelThresholds {
		if xp >= t {
			lv = i + 1
		}
	}
	if xp >= 1000 {
		lv = 5 + (xp-1000)/500
	}
	return lv
}

const (
	AchFirstDone     = "first_done"
	AchStreak7       = "streak_7"
	AchStreak30      = "streak_30"
	AchStreak100     = "streak_100"
	AchPerfectWeek   = "perfect_week"
	AchEarlyBird     = "early_bird"
	AchCompletionist = "completionist"
)

func AllCodes() []string {
	return []string{AchFirstDone, AchStreak7, AchStreak30, AchStreak100, AchPerfectWeek, AchEarlyBird, AchCompletionist}
}

type AchievementDef struct {
	Code        string
	Names       map[string]string
	ShieldBonus int
	XPBonus     int
}

var definitions = []AchievementDef{
	{AchFirstDone, map[string]string{"ru": "Первый шаг", "en": "First Step", "kz": "Бірінші қадам"}, 1, 0},
	{AchStreak7, map[string]string{"ru": "7-дневный воин", "en": "7-Day Warrior", "kz": "7 күндік жауынгер"}, 1, 0},
	{AchStreak30, map[string]string{"ru": "30-дневный чемпион", "en": "30-Day Champion", "kz": "30 күндік чемпион"}, 1, 100},
	{AchStreak100, map[string]string{"ru": "Легенда", "en": "Legend", "kz": "Аңыз"}, 2, 500},
	{AchPerfectWeek, map[string]string{"ru": "Идеальная неделя", "en": "Perfect Week", "kz": "Мінсіз апта"}, 1, 0},
	{AchEarlyBird, map[string]string{"ru": "Ранняя пташка", "en": "Early Bird", "kz": "Ерте тұрған"}, 0, 0},
	{AchCompletionist, map[string]string{"ru": "Перфекционист", "en": "Completionist", "kz": "Перфекционист"}, 0, 0},
}

var defsByCode = func() map[string]*AchievementDef {
	m := make(map[string]*AchievementDef, len(definitions))
	for i := range definitions {
		m[definitions[i].Code] = &definitions[i]
	}
	return m
}()

func GetDef(code string) (*AchievementDef, bool) { d, ok := defsByCode[code]; return d, ok }

func DisplayName(code, lang string) string {
	d, ok := defsByCode[code]
	if !ok {
		return code
	}
	if name, ok := d.Names[lang]; ok {
		return name
	}
	if name, ok := d.Names["en"]; ok {
		return name
	}
	return code
}

func RewardText(def *AchievementDef, lang string) string {
	switch {
	case def.ShieldBonus > 0 && def.XPBonus > 0:
		shield := map[string]string{"ru": "+%d щит(а)", "en": "+%d shield(s)", "kz": "+%d қалқан"}[lang]
		if shield == "" {
			shield = "+%d shield(s)"
		}
		return fmt.Sprintf(shield, def.ShieldBonus) + fmt.Sprintf(" · +%d XP", def.XPBonus)
	case def.ShieldBonus > 0:
		shield := map[string]string{"ru": "+%d щит(а) стрика", "en": "+%d streak shield(s)", "kz": "+%d серия қалқаны"}[lang]
		if shield == "" {
			shield = "+%d streak shield(s)"
		}
		return fmt.Sprintf(shield, def.ShieldBonus)
	case def.XPBonus > 0:
		return fmt.Sprintf("+%d XP", def.XPBonus)
	default:
		return "🏅 badge"
	}
}

func FormatAchievementLine(code string, unlockedAt interface{ Format(string) string }, lang string) string {
	name := DisplayName(code, lang)
	return fmt.Sprintf("🏆 %s — %s\n", name, unlockedAt.Format("02.01.2006"))
}
