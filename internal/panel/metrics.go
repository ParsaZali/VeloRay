package panel

import (
	"context"
	"maps"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type cpuMeter struct {
	mu          sync.Mutex
	total, idle float64
}

var onDemandCPU cpuMeter

func readCPU() (float64, float64) {
	raw, _ := os.ReadFile("/proc/stat")
	line := strings.SplitN(string(raw), "\n", 2)[0]
	f := strings.Fields(line)
	var total, idle float64
	if len(f) < 2 {
		return 0, 0
	}
	for i, v := range f[1:] {
		n, _ := strconv.ParseFloat(v, 64)
		if i < 8 {
			total += n
		}
		if i == 3 || i == 4 {
			idle += n
		}
	}
	return total, idle
}
func (m *cpuMeter) sample() (float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	total, idle := readCPU()
	if total <= 0 || total == m.total {
		return 0, false
	}
	previousTotal, previousIdle := m.total, m.idle
	m.total, m.idle = total, idle
	if previousTotal <= 0 || total < previousTotal || idle < previousIdle {
		return 0, false
	}
	return max(0, min(100, (1-(idle-previousIdle)/(total-previousTotal))*100)), true
}

func hostMetrics() Data {
	d, _ := collectHostMetrics(&onDemandCPU)
	return d
}

func collectHostMetrics(cpu *cpuMeter) (Data, bool) {
	host, _ := os.Hostname()
	d := Data{"hostname": host, "platform": runtime.GOOS + "/" + runtime.GOARCH, "go_version": runtime.Version(), "uptime_seconds": 0, "load_average": []float64{0, 0, 0}, "cpu_count": runtime.NumCPU(), "cpu_percent": float64(0)}
	raw, _ := os.ReadFile("/proc/uptime")
	if f := strings.Fields(string(raw)); len(f) > 0 {
		v, _ := strconv.ParseFloat(f[0], 64)
		d["uptime_seconds"] = int64(v)
	}
	raw, _ = os.ReadFile("/proc/loadavg")
	f := strings.Fields(string(raw))
	if len(f) >= 3 {
		values := []float64{}
		for _, s := range f[:3] {
			v, _ := strconv.ParseFloat(s, 64)
			values = append(values, v)
		}
		d["load_average"] = values
	}
	mem := map[string]int64{}
	raw, _ = os.ReadFile("/proc/meminfo")
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 {
			n, _ := strconv.ParseInt(f[1], 10, 64)
			mem[strings.TrimSuffix(f[0], ":")] = n * 1024
		}
	}
	total, available := mem["MemTotal"], mem["MemAvailable"]
	swap, free := mem["SwapTotal"], mem["SwapFree"]
	d["memory_total_bytes"] = total
	d["memory_available_bytes"] = available
	d["memory_used_bytes"] = max(0, total-available)
	d["memory_percent"] = percent(total-available, total)
	d["swap_total_bytes"] = swap
	d["swap_free_bytes"] = free
	d["swap_used_bytes"] = max(0, swap-free)
	d["swap_percent"] = percent(swap-free, swap)
	var st unix.Statfs_t
	if unix.Statfs("/", &st) == nil {
		total := int64(st.Blocks) * st.Bsize
		free := int64(st.Bavail) * st.Bsize
		d["disk_total_bytes"] = total
		d["disk_free_bytes"] = free
		d["disk_used_bytes"] = total - free
		d["disk_percent"] = percent(total-free, total)
	}
	usage, measured := cpu.sample()
	d["cpu_percent"] = usage
	return d, measured
}
func percent(a, b int64) float64 {
	if b <= 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}

const resourceSampleInterval = 5 * time.Second
const resourceHistoryLimit = 60

type resourceSample struct {
	At     time.Time `json:"at"`
	CPU    *float64  `json:"cpu"`
	Memory *float64  `json:"memory"`
	Swap   *float64  `json:"swap"`
	Disk   *float64  `json:"disk"`
}

type resourceMetrics struct {
	mu      sync.RWMutex
	cpu     cpuMeter
	latest  Data
	samples []resourceSample
}

func (m *resourceMetrics) record(at time.Time, d Data, cpuMeasured bool) {
	value := func(key string, available bool) *float64 {
		if v, ok := d[key].(float64); available && ok {
			return &v
		}
		return nil
	}
	sample := resourceSample{
		At:     at.UTC(),
		CPU:    value("cpu_percent", cpuMeasured),
		Memory: value("memory_percent", num(d, "memory_total_bytes") > 0),
		Swap:   value("swap_percent", num(d, "memory_total_bytes") > 0),
		Disk:   value("disk_percent", num(d, "disk_total_bytes") > 0),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latest = d
	if len(m.samples) == resourceHistoryLimit {
		copy(m.samples, m.samples[1:])
		m.samples[len(m.samples)-1] = sample
	} else {
		m.samples = append(m.samples, sample)
	}
}

func (m *resourceMetrics) collect() {
	d, cpuMeasured := collectHostMetrics(&m.cpu)
	m.record(time.Now(), d, cpuMeasured)
}

func (m *resourceMetrics) run(ctx context.Context) {
	ticker := time.NewTicker(resourceSampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.collect()
		}
	}
}

func (m *resourceMetrics) snapshot(includeHistory bool) Data {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d := maps.Clone(m.latest)
	if d != nil && includeHistory {
		d["resource_history"] = append([]resourceSample{}, m.samples...)
	}
	return d
}

func (s *Server) systemInfo(ctx context.Context, resourcesOnly bool) (Data, error) {
	d := s.resources.snapshot(resourcesOnly)
	if d == nil {
		d = hostMetrics()
	}
	if !resourcesOnly {
		var users, admins int
		if e := s.Store.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE data->>'is_active'='true' AND data->>'is_superuser'='true') FROM vr_users`).Scan(&users, &admins); e != nil {
			return nil, e
		}
		d["users_total"] = users
		d["admins_total"] = admins
	}
	runtime := s.panelRuntime(time.Now())
	d["panel_uptime_seconds"] = runtime["uptime_seconds"]
	d["panel_started_at"] = runtime["started_at"]
	d["panel_version"] = Version
	d["server_time"] = stamp(time.Now())
	return d, nil
}

func (s *Server) panelRuntime(now time.Time) Data {
	started := s.StartedAt
	if started.IsZero() {
		started = panelProcessStartedAt
	}
	return Data{"uptime_seconds": max(int64(0), int64(now.Sub(started).Seconds())), "started_at": stamp(started)}
}
