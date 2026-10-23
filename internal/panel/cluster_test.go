package panel

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestIntegrationStandbyWorkerTakesOverWithoutConcurrentJobs(t *testing.T) {
	s, ctx := integration(t)
	second := &Server{Store: s.Store, Logger: s.Logger}
	ctx1, cancel1 := context.WithCancel(ctx)
	ctx2, cancel2 := context.WithCancel(ctx)
	defer cancel1()
	defer cancel2()
	var running, maxRunning, starts atomic.Int32
	started := make(chan int, 4)
	done1, done2 := make(chan struct{}), make(chan struct{})
	work := func(id int) func(context.Context) {
		return func(c context.Context) {
			n := running.Add(1)
			for previous := maxRunning.Load(); n > previous && !maxRunning.CompareAndSwap(previous, n); previous = maxRunning.Load() {
			}
			starts.Add(1)
			started <- id
			<-c.Done()
			running.Add(-1)
		}
	}
	go func() { defer close(done1); s.leadershipLoop(ctx1, 30*time.Millisecond, work(1)) }()
	var first int
	select {
	case first = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first worker did not acquire leadership")
	}
	if first != 1 {
		t.Fatal(first)
	}
	go func() { defer close(done2); second.leadershipLoop(ctx2, 30*time.Millisecond, work(2)) }()
	// Seeing both registered instances proves the second contender has checked
	// the shared database, rather than relying on an arbitrary sleep.
	deadline := time.Now().Add(5 * time.Second)
	registered := false
	for time.Now().Before(deadline) {
		var n int
		if e := s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_panel_instances`).Scan(&n); e != nil {
			t.Fatal(e)
		}
		if n == 2 {
			registered = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !registered || starts.Load() != 1 {
		t.Fatal("standby started jobs", registered, starts.Load())
	}
	cancel1()
	select {
	case <-done1:
	case <-time.After(5 * time.Second):
		t.Fatal("old worker did not stop")
	}
	select {
	case next := <-started:
		if next != 2 {
			t.Fatal(next)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("standby did not take over")
	}
	if maxRunning.Load() != 1 {
		t.Fatal("overlapping leaders", maxRunning.Load())
	}
	cancel2()
	<-done2
}
