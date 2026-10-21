package nativecore

import "errors"

func (e *Engine) OwnsUDP(addr string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.awgs[addr] != nil
}

// All new sockets and stacks are prepared before changing existing peer sets.
// Typed validation excludes UAPI commands, host routing and arbitrary devices.
func (e *Engine) applyAWG(inbounds []Inbound) error {
	e.mu.RLock()
	old := e.awgs
	e.mu.RUnlock()
	next := map[string]*awgRuntime{}
	created := []*awgRuntime{}
	configs := map[string]Inbound{}
	cleanup := func() {
		for _, n := range created {
			n.close()
		}
	}
	for _, in := range inbounds {
		if in.Protocol != "amneziawg" {
			continue
		}
		key := address(in)
		configs[key] = in
		n := old[key]
		if n == nil {
			var err error
			n, err = newAWG(e, in)
			if err != nil {
				cleanup()
				return err
			}
			created = append(created, n)
		}
		next[key] = n
	}
	changed := []string{}
	for key, n := range next {
		if old[key] == nil {
			continue
		}
		in := configs[key]
		before := n.config
		if err := n.dev.IpcSet(awgUAPI(in, &before, false)); err != nil {
			// Restore the failed peer set and any earlier mutation.
			_ = n.dev.IpcSet(awgUAPI(before, &in, false))
			for _, prior := range changed {
				v := next[prior]
				after := configs[prior]
				_ = v.dev.IpcSet(awgUAPI(v.config, &after, false))
			}
			cleanup()
			return errors.New("AmneziaWG peer update failed; previous configuration retained")
		}
		changed = append(changed, key)
	}
	for key, n := range next {
		n.setUsers(configs[key])
		n.tun.setMTU(configs[key].AWG.MTU)
		n.mu.Lock()
		n.active = true
		n.mu.Unlock()
	}
	e.mu.Lock()
	e.awgs = next
	e.mu.Unlock()
	for key, n := range old {
		if next[key] != n {
			n.close()
		}
	}
	return nil
}
