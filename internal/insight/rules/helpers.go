package rules

import "time"

// isoWeekStart returns the Monday of the ISO week that contains t.
func isoWeekStart(t time.Time) time.Time {
	t = t.UTC().Truncate(24 * time.Hour)
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return t.AddDate(0, 0, -(weekday - 1))
}
