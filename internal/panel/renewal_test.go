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
