package telegram

import (
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/saidakmal/habbit-tracker-bot/internal/i18n"
)

func mainNavKeyboard(lang i18n.Lang) tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(i18n.T(lang, "nav.today")),
			tgbotapi.NewKeyboardButton(i18n.T(lang, "nav.my_habits")),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(i18n.T(lang, "nav.add_habit")),
			tgbotapi.NewKeyboardButton(i18n.T(lang, "nav.stats")),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(i18n.T(lang, "nav.settings")),
		),
	)
	kb.ResizeKeyboard = true
	return kb
}

func (h *Handler) sendMainNav(chatID int64, lang i18n.Lang) {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "nav.menu_hint"))
	m.ReplyMarkup = mainNavKeyboard(lang)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send main nav", zap.Error(err))
	}
}

func (h *Handler) sendTimezoneKeyboard(chatID int64, lang i18n.Lang, callbackPrefix string) {
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(commonTimezones); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(commonTimezones[i].Label, callbackPrefix+commonTimezones[i].Value),
		}
		if i+1 < len(commonTimezones) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(commonTimezones[i+1].Label, callbackPrefix+commonTimezones[i+1].Value))
		}
		rows = append(rows, row)
	}
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "timezone.choose"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send timezone keyboard", zap.Error(err))
	}
}

func onboardTemplateKeyboard(lang i18n.Lang) tgbotapi.InlineKeyboardMarkup {
	base := templateKeyboard(lang)
	base.InlineKeyboard = append(base.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "onboarding.skip_btn"), "onboard_skip:1"),
	))
	return base
}

func templateKeyboard(lang i18n.Lang) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "template.water"), "template:water"),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "template.exercise"), "template:exercise"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "template.read"), "template:read"),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "template.meditate"), "template:meditate"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "template.sleep"), "template:sleep"),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "template.custom"), "template:custom"),
		),
	)
}

func (h *Handler) sendIntervalKeyboard(chatID int64, lang i18n.Lang) error {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_interval"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(formatInterval(30, lang), "interval:30"),
			tgbotapi.NewInlineKeyboardButtonData(formatInterval(60, lang), "interval:60"),
			tgbotapi.NewInlineKeyboardButtonData(formatInterval(120, lang), "interval:120"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(formatInterval(180, lang), "interval:180"),
			tgbotapi.NewInlineKeyboardButtonData(formatInterval(1440, lang), "interval:1440"),
		),
	)
	_, err := h.api.Send(m)
	return err
}

func hourButtons(prefix string, hours []int) [][]tgbotapi.InlineKeyboardButton {
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(hours); i += 4 {
		end := i + 4
		if end > len(hours) {
			end = len(hours)
		}
		var row []tgbotapi.InlineKeyboardButton
		for _, h := range hours[i:end] {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("%d:00", h),
				fmt.Sprintf("%s:%d", prefix, h),
			))
		}
		rows = append(rows, row)
	}
	return rows
}

func (h *Handler) sendStartHourKeyboard(chatID int64, lang i18n.Lang) error {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_start"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(hourButtons("start_hour", []int{5, 6, 7, 8, 9, 10, 11, 12})...)
	_, err := h.api.Send(m)
	return err
}

func (h *Handler) sendEndHourKeyboard(chatID int64, lang i18n.Lang, minHour int) error {
	all := []int{14, 16, 18, 20, 21, 22, 23}
	var valid []int
	for _, hr := range all {
		if hr > minHour {
			valid = append(valid, hr)
		}
	}
	if len(valid) == 0 {
		h.send(chatID, i18n.T(lang, "error.generic"))
		h.clearState(chatID)
		return nil
	}
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_end"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(hourButtons("end_hour", valid)...)
	_, err := h.api.Send(m)
	return err
}

var goalDayOptions = []int{21, 30, 66, 100, 0}

func goalKeyboardRows(lang i18n.Lang, callbackPrefix func(days int) string) [][]tgbotapi.InlineKeyboardButton {
	row1 := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "goal.days_btn", goalDayOptions[0]), callbackPrefix(goalDayOptions[0])),
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "goal.days_btn", goalDayOptions[1]), callbackPrefix(goalDayOptions[1])),
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "goal.days_btn", goalDayOptions[2]), callbackPrefix(goalDayOptions[2])),
	)
	row2 := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "goal.days_btn", goalDayOptions[3]), callbackPrefix(goalDayOptions[3])),
		tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "goal.no_goal"), callbackPrefix(goalDayOptions[4])),
	)
	return [][]tgbotapi.InlineKeyboardButton{row1, row2}
}

func (h *Handler) sendGoalKeyboard(chatID int64, lang i18n.Lang) error {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_goal"))
	rows := goalKeyboardRows(lang, func(days int) string {
		return fmt.Sprintf("add_goal:%d", days)
	})
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err := h.api.Send(m)
	return err
}

func (h *Handler) sendEditIntervalKeyboard(chatID, habitID int64, lang i18n.Lang) {
	id := fmt.Sprintf("%d", habitID)
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_interval"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(formatInterval(30, lang), "edit_interval:"+id+":30"),
			tgbotapi.NewInlineKeyboardButtonData(formatInterval(60, lang), "edit_interval:"+id+":60"),
			tgbotapi.NewInlineKeyboardButtonData(formatInterval(120, lang), "edit_interval:"+id+":120"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(formatInterval(180, lang), "edit_interval:"+id+":180"),
			tgbotapi.NewInlineKeyboardButtonData(formatInterval(1440, lang), "edit_interval:"+id+":1440"),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send edit interval keyboard", zap.Error(err))
	}
}

func (h *Handler) sendEditStartHourKeyboard(chatID, habitID int64, lang i18n.Lang) {
	id := fmt.Sprintf("%d", habitID)
	prefix := "edit_start:" + id
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_start"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(hourButtons(prefix, []int{5, 6, 7, 8, 9, 10, 11, 12})...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send edit start hour keyboard", zap.Error(err))
	}
}

func (h *Handler) sendEditEndHourKeyboard(chatID, habitID int64, minHour int, lang i18n.Lang) {
	id := fmt.Sprintf("%d", habitID)
	prefix := "edit_end:" + id
	all := []int{14, 16, 18, 20, 21, 22, 23}
	var valid []int
	for _, hr := range all {
		if hr > minHour {
			valid = append(valid, hr)
		}
	}
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_end"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(hourButtons(prefix, valid)...)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send edit end hour keyboard", zap.Error(err))
	}
}

func (h *Handler) resendCurrentStep(chatID int64, lang i18n.Lang, state *convState) error {
	switch state.Step {
	case stepIdle:
		m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.choose_template"))
		m.ReplyMarkup = templateKeyboard(lang)
		_, err := h.api.Send(m)
		return err
	case stepAwaitInterval:
		return h.sendIntervalKeyboard(chatID, lang)
	case stepAwaitStartHour:
		return h.sendStartHourKeyboard(chatID, lang)
	case stepAwaitEndHour:
		return h.sendEndHourKeyboard(chatID, lang, state.StartHour+1)
	case stepAwaitGoal:
		return h.sendGoalKeyboard(chatID, lang)
	case stepAwaitMotivation:
		return h.sendMotivationPrompt(chatID, lang)
	default:
		return nil
	}
}

func (h *Handler) sendMotivationPrompt(chatID int64, lang i18n.Lang) error {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "habit.enter_motivation"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "habit.motivation_skip"), "add_motivation:skip"),
		),
	)
	_, err := h.api.Send(m)
	return err
}

func (h *Handler) sendMoodPrompt(chatID int64, lang i18n.Lang) {
	m := tgbotapi.NewMessage(chatID, i18n.T(lang, "mood.check_in"))
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.great"), "mood:3"),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.okay"), "mood:2"),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "mood.tough"), "mood:1"),
		),
	)
	if _, err := h.api.Send(m); err != nil {
		h.logger.Error("send mood prompt", zap.Error(err))
	}
}

func doneMessage(name string, streak, goalDays int, lang i18n.Lang) string {
	if goalDays > 0 && streak > 0 {
		return i18n.T(lang, "habit.done_goal", name, streak, goalDays)
	}
	if streak > 0 {
		return i18n.T(lang, "habit.done_streak", name, streak)
	}
	return i18n.T(lang, "habit.done_simple")
}

func formatInterval(minutes int, lang string) string {
	switch {
	case minutes >= 1440:
		return i18n.T(lang, "interval.daily")
	case minutes == 60:
		return i18n.T(lang, "interval.hourly")
	case minutes >= 60:
		return i18n.T(lang, "interval.every_n_hours", minutes/60)
	default:
		return i18n.T(lang, "interval.every_n_min", minutes)
	}
}

func progressBar(done, total int) string {
	const width = 10
	if total == 0 {
		return strings.Repeat("░", width)
	}
	filled := done * width / total
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func buildHeatmap(habitName string, from, to time.Time, doneSet map[string]bool, lang string) string {
	weekdays := []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	start := from
	for start.Weekday() != time.Monday {
		start = start.AddDate(0, 0, -1)
	}
	grid := [7][4]string{}
	for row := 0; row < 7; row++ {
		for col := 0; col < 4; col++ {
			day := start.AddDate(0, 0, col*7+row)
			if day.After(to) {
				grid[row][col] = " "
				continue
			}
			if doneSet[day.Format("2006-01-02")] {
				grid[row][col] = "■"
			} else {
				grid[row][col] = "□"
			}
		}
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "history.header", habitName))
	sb.WriteString("\n")
	for row := 0; row < 7; row++ {
		sb.WriteString(weekdays[row] + " ")
		for col := 0; col < 4; col++ {
			sb.WriteString(grid[row][col] + " ")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(i18n.T(lang, "history.legend"))
	return sb.String()
}
