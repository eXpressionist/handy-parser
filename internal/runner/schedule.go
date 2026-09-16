package runner

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type DailySchedule struct {
	minutes []int
}

func ParseDailySchedule(raw string) (DailySchedule, error) {
	parts := strings.Split(raw, ",")
	seen := make(map[int]bool, len(parts))
	minutes := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		fields := strings.Split(part, ":")
		if len(fields) != 2 {
			return DailySchedule{}, fmt.Errorf("invalid check time %q; use HH:MM", part)
		}
		hour, hourErr := strconv.Atoi(fields[0])
		minute, minuteErr := strconv.Atoi(fields[1])
		if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return DailySchedule{}, fmt.Errorf("invalid check time %q; use HH:MM", part)
		}
		value := hour*60 + minute
		if !seen[value] {
			seen[value] = true
			minutes = append(minutes, value)
		}
	}
	if len(minutes) == 0 {
		return DailySchedule{}, fmt.Errorf("check schedule is empty")
	}
	sort.Ints(minutes)
	return DailySchedule{minutes: minutes}, nil
}

func (s DailySchedule) Next(after time.Time, location *time.Location) time.Time {
	local := after.In(location)
	for dayOffset := 0; ; dayOffset++ {
		day := local.AddDate(0, 0, dayOffset)
		for _, value := range s.minutes {
			candidate := time.Date(day.Year(), day.Month(), day.Day(), value/60, value%60, 0, 0, location)
			if candidate.After(after) {
				return candidate
			}
		}
	}
}

func (r *Runner) RunSchedule(ctx context.Context, schedule DailySchedule, location *time.Location) {
	for {
		next := schedule.Next(time.Now(), location)
		r.logger.Info("next scheduled check", "at", next.Format(time.RFC3339))
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
		summary, err := r.Run(ctx)
		if err != nil {
			r.logger.Error("scheduled check failed", "error", err)
			continue
		}
		r.logger.Info("scheduled check finished", "checked", summary.Checked, "changed", summary.Changed, "errors", summary.Errors, "skipped", summary.Skipped)
	}
}
