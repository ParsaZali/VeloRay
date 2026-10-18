package panel

import (
	"errors"
	"time"
	_ "time/tzdata"
)

func renewalSpec(c Data) (string, int64, *time.Location, error) {
	period := str(c, "renewal_period")
	every := num(c, "renewal_every")
	zone := fallback(str(c, "renewal_timezone"), "UTC")
	if period == "" || period == "none" {
		period = "day"
		every = num(c, "renewal_interval_days")
		zone = "UTC"
	}
	if !oneOf(period, "hour", "day", "week", "month") || every < 0 || every > 3650 {
		return "", 0, nil, errors.New("invalid renewal period or interval")
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return "", 0, nil, errors.New("renewal timezone must be an IANA timezone")
	}
	return period, every, loc, nil
}
func calendarRenewal(c Data, anchor, now time.Time) (time.Time, error) {
	period, every, loc, e := renewalSpec(c)
	if e != nil {
		return time.Time{}, e
	}
	if every == 0 {
		return time.Time{}, nil
	}
	anchor = anchor.In(loc)
	now = now.In(loc)
	if period == "hour" {
		interval := time.Duration(every) * time.Hour
		steps := int64(1)
		if !anchor.After(now) {
			steps = int64(now.Sub(anchor)/interval) + 1
		}
		return anchor.Add(time.Duration(steps) * interval).UTC(), nil
	}
	if period == "month" {
		day := num(c, "_renewal_day")
		if day < 1 || day > 31 {
			day = int64(anchor.Day())
			c["_renewal_day"] = day
		}
		months := int64((now.Year()-anchor.Year())*12 + int(now.Month()-anchor.Month()))
		step := int64(1)
		if months > 0 {
			step = months / every
			if step < 1 {
				step = 1
			}
		}
		candidate := func(step int64) time.Time {
			first := time.Date(anchor.Year(), anchor.Month()+time.Month(step*every), 1, anchor.Hour(), anchor.Minute(), anchor.Second(), anchor.Nanosecond(), loc)
			last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, loc).Day()
			d := int(day)
			if d > last {
				d = last
			}
			return time.Date(first.Year(), first.Month(), d, anchor.Hour(), anchor.Minute(), anchor.Second(), anchor.Nanosecond(), loc)
		}
		next := candidate(step)
		if !next.After(now) {
			next = candidate(step + 1)
		}
		return next.UTC(), nil
	}
	days := every
	if period == "week" {
		days *= 7
	}
	// Compare calendar dates in UTC solely for ordinal arithmetic; construction in
	// the requested location preserves wall-clock renewal time across DST.
	ordinal := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	elapsed := int64(ordinal(now).Sub(ordinal(anchor)) / (24 * time.Hour))
	steps := int64(1)
	if elapsed > 0 {
		steps = elapsed / days
		if steps < 1 {
			steps = 1
		}
	}
	next := anchor.AddDate(0, 0, int(steps*days))
	if !next.After(now) {
		next = anchor.AddDate(0, 0, int((steps+1)*days))
	}
	return next.UTC(), nil
}
func scheduleRenewal(c Data, now time.Time) error {
	_, every, _, e := renewalSpec(c)
	if e != nil {
		return e
	}
	if every == 0 {
		c["next_renewal_at"] = nil
		return nil
	}
	if str(c, "next_renewal_at") == "" {
		next, e := calendarRenewal(c, now, now)
		if e != nil {
			return e
		}
		c["next_renewal_at"] = stamp(next)
	}
	return nil
}
