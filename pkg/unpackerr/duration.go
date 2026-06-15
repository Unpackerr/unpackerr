package unpackerr

import (
	"strconv"
	"strings"
	"time"
)

const (
	durationPartsLimit = 3
	daysPerWeek        = 7
	daysPerYear        = 365
	hoursPerDay        = 24
)

func formatDuration(d time.Duration) string {
	d = d.Abs()
	units := []struct {
		name     string
		duration time.Duration
	}{
		{name: "year", duration: daysPerYear * hoursPerDay * time.Hour},
		{name: "week", duration: daysPerWeek * hoursPerDay * time.Hour},
		{name: "day", duration: hoursPerDay * time.Hour},
		{name: "hour", duration: time.Hour},
		{name: "minute", duration: time.Minute},
		{name: "second", duration: time.Second},
		{name: "millisecond", duration: time.Millisecond},
		{name: "microsecond", duration: time.Microsecond},
	}

	parts := make([]string, 0, durationPartsLimit)
	remaining := d

	for _, unit := range units {
		count := int64(remaining / unit.duration)
		if count == 0 {
			continue
		}

		parts = append(parts, formatDurationPart(count, unit.name))
		remaining -= time.Duration(count) * unit.duration

		if len(parts) == durationPartsLimit {
			break
		}
	}

	if len(parts) == 0 {
		return "0 seconds"
	}

	return strings.Join(parts, " ")
}

func formatDurationPart(count int64, name string) string {
	if count == 1 {
		return strconv.FormatInt(count, 10) + " " + name
	}

	return strconv.FormatInt(count, 10) + " " + name + "s"
}
