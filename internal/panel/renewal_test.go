package panel

import (
	"testing"
	"time"
)

func TestCalendarRenewalKeepsMonthDayAndLocalWallClock(t *testing.T) {
	parse := func(s string) time.Time {
		v, e := time.Parse(time.RFC3339, s)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	c := Data{"renewal_period": "month", "renewal_every": 1, "renewal_timezone": "UTC"}
	jan := parse("2028-01-31T12:00:00Z")
	feb, e := calendarRenewal(c, jan, jan)
	if e != nil || stamp(feb) != "2028-02-29T12:00:00Z" {
		t.Fatal(feb, e)
	}
	march, e := calendarRenewal(c, feb, feb)
	if e != nil || stamp(march) != "2028-03-31T12:00:00Z" {
		t.Fatal("lost original month day", march, e)
	}
	future, e := calendarRenewal(c, march, parse("2030-05-15T00:00:00Z"))
	if e != nil || stamp(future) != "2030-05-31T12:00:00Z" {
		t.Fatal("missed periods not skipped", future, e)
	}
	c = Data{"renewal_period": "day", "renewal_every": 1, "renewal_timezone": "America/New_York"}
	before := parse("2028-03-11T17:00:00Z")
	after, e := calendarRenewal(c, before, before)
	if e != nil || after.Sub(before) != 23*time.Hour {
		t.Fatal("DST wall clock shifted", before, after, e)
	}
	c = Data{"renewal_interval_days": 7}
	next, e := calendarRenewal(c, jan, jan.Add(20*24*time.Hour))
	if e != nil || !next.Equal(jan.Add(21*24*time.Hour)) {
		t.Fatal("legacy renewal changed", next, e)
	}
	for _, bad := range []Data{{"renewal_period": "month", "renewal_every": -1}, {"renewal_period": "month", "renewal_every": 1, "renewal_timezone": "invalid/zone"}} {
		if e := scheduleRenewal(bad, jan); e == nil {
			t.Fatal("invalid renewal accepted", bad)
		}
	}
}

func TestRenewalSpringGapAndAncientAnchor(t *testing.T) {
	parse := func(s string) time.Time {
		v, e := time.Parse(time.RFC3339, s)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	c := Data{"renewal_period": "day", "renewal_every": 1, "renewal_timezone": "America/New_York"}
	before := parse("2028-03-11T07:30:00Z")
	gap, e := calendarRenewal(c, before, before)
	if e != nil || stamp(gap) != "2028-03-12T07:30:00Z" {
		t.Fatal("DST gap did not advance", gap, e)
	}
	next, e := calendarRenewal(c, gap, gap)
	if e != nil || stamp(next) != "2028-03-13T06:30:00Z" {
		t.Fatal("DST gap permanently shifted renewal clock", next, e)
	}
	for _, period := range []string{"hour", "day", "week"} {
		c = Data{"renewal_period": period, "renewal_every": 1, "renewal_timezone": "UTC"}
		now := parse("2030-05-15T12:00:00Z")
		next, e := calendarRenewal(c, parse("1000-01-01T00:00:00Z"), now)
		if e != nil || !next.After(now) || next.Sub(now) > 7*24*time.Hour {
			t.Fatal("elapsed duration overflowed", period, next, e)
		}
	}
}

func TestRenewalIntervalEditReanchorsWithoutDiscardingExplicitInstant(t *testing.T) {
	previous := Data{"renewal_period": "month", "renewal_every": 1, "renewal_timezone": "UTC", "next_renewal_at": "2030-01-31T12:00:00Z", "_renewal_day": 31, "_renewal_clock": "12:00:00"}
	d := clone(previous)
	d["renewal_every"] = 2
	reanchorRenewal(d, previous, Data{"renewal_every": 2})
	if d["next_renewal_at"] != nil || str(d, "_renewal_clock") != "" || num(d, "_renewal_day") != 0 {
		t.Fatal("numeric interval edit not detected", d)
	}
	d = clone(previous)
	d["renewal_every"] = 2
	d["next_renewal_at"] = "2030-02-20T14:00:00Z"
	reanchorRenewal(d, previous, Data{"renewal_every": 2, "next_renewal_at": d["next_renewal_at"]})
	if str(d, "next_renewal_at") != "2030-02-20T14:00:00Z" {
		t.Fatal("explicit instant discarded", d)
	}
}
