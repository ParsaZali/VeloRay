package panel

import (
	"context"
	"testing"
	"time"
)

func TestResourceHistoryUsesMeasuredValuesAndKeepsOnlyRecentSamples(t *testing.T) {
	var server Server
	start := time.Date(2026, 11, 26, 14, 0, 0, 0, time.UTC)
	server.resources.record(start, Data{
		"cpu_percent": 0.0, "memory_total_bytes": int64(100), "memory_percent": 27.5, "swap_percent": 0.0,
	}, false)
	first := server.resources.snapshot(true)["resource_history"].([]resourceSample)[0]
	if first.CPU != nil || first.Disk != nil || first.Swap == nil || *first.Swap != 0 {
		t.Fatalf("unavailable and zero measurements must be distinct: %+v", first)
	}
	for i := 1; i <= resourceHistoryLimit+1; i++ {
		server.resources.record(start.Add(time.Duration(i)*resourceSampleInterval), Data{
			"cpu_percent": float64(i), "memory_total_bytes": int64(100), "memory_percent": 27.5, "swap_percent": 0.0,
			"disk_total_bytes": int64(100), "disk_percent": 49.9,
		}, true)
	}
	info, err := server.systemInfo(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	samples := info["resource_history"].([]resourceSample)
	if len(samples) != resourceHistoryLimit {
		t.Fatalf("unexpected resource history length: %d", len(samples))
	}
	if !samples[0].At.Equal(start.Add(2 * resourceSampleInterval)) {
		t.Fatalf("unexpected first sample: %v", samples[0].At)
	}
	last := samples[len(samples)-1]
	if last.CPU == nil || *last.CPU != float64(resourceHistoryLimit+1) || last.Disk == nil || *last.Disk != 49.9 {
		t.Fatalf("latest measurements missing: %+v", last)
	}
	if _, ok := info["users_total"]; ok {
		t.Fatal("resource response unexpectedly fetched account counts")
	}
	if _, ok := server.resources.snapshot(false)["resource_history"]; ok {
		t.Fatal("regular system info should not include the history")
	}
}
