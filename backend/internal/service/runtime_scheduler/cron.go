package runtimescheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const maxCronSearchMinutes = 60 * 24 * 366 * 2

type cronSpec struct {
	minutes     [60]bool
	hours       [24]bool
	days        [32]bool
	months      [13]bool
	weekdays    [7]bool
	dayWildcard bool
	dowWildcard bool
}

func nextCron(expr string, after time.Time) (time.Time, error) {
	spec, err := parseCronSpec(expr)
	if err != nil {
		return time.Time{}, err
	}
	base := after.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < maxCronSearchMinutes; i++ {
		candidate := base.Add(time.Duration(i) * time.Minute)
		if spec.match(candidate) {
			return candidate, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot find next run time within %d minutes", maxCronSearchMinutes)
}

func parseCronSpec(expr string) (*cronSpec, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return nil, fmt.Errorf("expected 5 fields")
	}
	spec := &cronSpec{}
	if err := parseCronField(parts[0], 0, 59, spec.minutes[:], nil); err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}
	if err := parseCronField(parts[1], 0, 23, spec.hours[:], nil); err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}
	spec.dayWildcard = parts[2] == "*"
	if err := parseCronField(parts[2], 1, 31, spec.days[:], nil); err != nil {
		return nil, fmt.Errorf("invalid day-of-month field: %w", err)
	}
	if err := parseCronField(parts[3], 1, 12, spec.months[:], nil); err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}
	spec.dowWildcard = parts[4] == "*"
	if err := parseCronField(parts[4], 0, 7, nil, func(v int) {
		if v == 7 {
			spec.weekdays[0] = true
			return
		}
		spec.weekdays[v] = true
	}); err != nil {
		return nil, fmt.Errorf("invalid day-of-week field: %w", err)
	}
	return spec, nil
}

func parseCronField(field string, min, max int, slots []bool, setter func(v int)) error {
	setValue := func(v int) {
		if setter != nil {
			setter(v)
			return
		}
		slots[v] = true
	}
	if field == "*" {
		for i := min; i <= max; i++ {
			setValue(i)
		}
		return nil
	}
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("empty segment")
		}
		step := 1
		base := part
		if strings.Contains(part, "/") {
			items := strings.Split(part, "/")
			if len(items) != 2 {
				return fmt.Errorf("invalid step segment %q", part)
			}
			base = strings.TrimSpace(items[0])
			parsedStep, err := strconv.Atoi(strings.TrimSpace(items[1]))
			if err != nil || parsedStep <= 0 {
				return fmt.Errorf("invalid step value in %q", part)
			}
			step = parsedStep
		}
		start, end := min, max
		switch {
		case base == "" || base == "*":
		case strings.Contains(base, "-"):
			bounds := strings.Split(base, "-")
			if len(bounds) != 2 {
				return fmt.Errorf("invalid range %q", base)
			}
			left, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil {
				return fmt.Errorf("invalid range start %q", base)
			}
			right, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil {
				return fmt.Errorf("invalid range end %q", base)
			}
			start, end = left, right
		default:
			single, err := strconv.Atoi(base)
			if err != nil {
				return fmt.Errorf("invalid value %q", base)
			}
			start, end = single, single
		}
		if start < min || end > max || start > end {
			return fmt.Errorf("value out of range %d-%d in %q", min, max, part)
		}
		for value := start; value <= end; value += step {
			setValue(value)
		}
	}
	return nil
}

func (c *cronSpec) match(ts time.Time) bool {
	if !c.minutes[ts.Minute()] || !c.hours[ts.Hour()] || !c.months[int(ts.Month())] {
		return false
	}
	dayMatch := c.days[ts.Day()]
	dowMatch := c.weekdays[int(ts.Weekday())]
	if c.dayWildcard && c.dowWildcard {
		return dayMatch && dowMatch
	}
	if c.dayWildcard {
		return dowMatch
	}
	if c.dowWildcard {
		return dayMatch
	}
	return dayMatch || dowMatch
}
