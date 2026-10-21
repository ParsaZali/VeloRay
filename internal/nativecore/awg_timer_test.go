package nativecore

import (
	"sync"
	"testing"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/device"
)

// A keepalive timer can expire while concurrent receive/send paths reschedule
// or cancel it. The upstream pending-duration write must share the same lock.
func TestAmneziaWGConcurrentTimerReschedule(t *testing.T) {
	timer := new(device.Peer).NewTimer(func(*device.Peer, time.Duration) {})
	defer timer.DelSync()
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 2000 {
				timer.Mod(time.Nanosecond)
				_ = timer.IsPending()
				timer.Del()
			}
		})
	}
	workers.Wait()
	timer.DelSync()
	if timer.IsPending() {
		t.Fatal("timer remained pending after synchronous cancellation")
	}
}
