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
		seconds := every * 3600
		steps := int64(1)
		if !anchor.After(now) {
			elapsed := now.Unix() - anchor.Unix()
			if now.Nanosecond() < anchor.Nanosecond() {
				elapsed--
			}
			steps = elapsed/seconds + 1
		}
		return time.Unix(anchor.Unix()+steps*seconds, int64(anchor.Nanosecond())).UTC(), nil
	}
	clock, e := time.Parse("15:04:05.999999999", str(c, "_renewal_clock"))
	if e != nil {
		clock = anchor
		c["_renewal_clock"] = anchor.Format("15:04:05.999999999")
	}
	wall := func(year int, month time.Month, day int) time.Time {
		target := time.Date(year, month, day, clock.Hour(), clock.Minute(), clock.Second(), clock.Nanosecond(), loc)
		wanted := time.Date(year, month, day, clock.Hour(), clock.Minute(), clock.Second(), clock.Nanosecond(), time.UTC)
		actual := time.Date(target.Year(), target.Month(), target.Day(), target.Hour(), target.Minute(), target.Second(), target.Nanosecond(), time.UTC)
		// A nonexistent wall time advances by the DST gap for this occurrence.
		// Keep the original clock for the following day's/month's renewal.
		if actual.Before(wanted) {
			target = target.Add(wanted.Sub(actual))
		}
		return target
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
			first := time.Date(anchor.Year(), anchor.Month()+time.Month(step*every), 1, 12, 0, 0, 0, loc)
			last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, loc).Day()
			d := int(day)
			if d > last {
				d = last
			}
			return wall(first.Year(), first.Month(), d)
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
	ordinal := func(t time.Time) int64 {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Unix() / 86400
	}
	elapsed := ordinal(now) - ordinal(anchor)
	steps := int64(1)
	if elapsed > 0 {
		steps = elapsed / days
		if steps < 1 {
			steps = 1
		}
	}
	next := wall(anchor.Year(), anchor.Month(), anchor.Day()+int(steps*days))
	if !next.After(now) {
		next = wall(anchor.Year(), anchor.Month(), anchor.Day()+int((steps+1)*days))
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

func reanchorRenewal(d, previous, input Data) {
	changed := false
	for _, k := range []string{"renewal_interval_days", "renewal_period", "renewal_every", "renewal_timezone"} {
		if (oneOf(k, "renewal_every", "renewal_interval_days") && num(d, k) != num(previous, k)) || (!oneOf(k, "renewal_every", "renewal_interval_days") && str(d, k) != str(previous, k)) {
			changed = true
		}
	}
	_, explicit := input["next_renewal_at"]
	if changed || explicit {
		d["_renewal_day"], d["_renewal_clock"] = 0, ""
		if !explicit {
			d["next_renewal_at"] = nil
		}
	}
}
