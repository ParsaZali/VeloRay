package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
)

// Adding authenticated users is safe without replacing listeners. Removals and
// credential changes retain restart semantics so quota/expiry enforcement also
// terminates old sessions. No request may enable the API on a public address.
func liveUserAdditions(before, after []byte) ([]byte, bool) {
	var old, next map[string]any
	if json.Unmarshal(before, &old) != nil || json.Unmarshal(after, &next) != nil {
		return nil, false
	}
	api, _ := old["api"].(map[string]any)
	services, _ := api["services"].([]any)
	handler := false
	for _, v := range services {
		if v == "HandlerService" {
			handler = true
		}
	}
	if !handler {
		return nil, false
	}
	oldIns, _ := old["inbounds"].([]any)
	newIns, _ := next["inbounds"].([]any)
	if len(oldIns) != len(newIns) {
		return nil, false
	}
	additions := []any{}
	for index, raw := range oldIns {
		o, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		n, ok := newIns[index].(map[string]any)
		if !ok {
			return nil, false
		}
		protocol, _ := o["protocol"].(string)
		if protocol != "vless" && protocol != "vmess" && protocol != "trojan" {
			if !reflect.DeepEqual(o, n) {
				return nil, false
			}
			continue
		}
		os, ok := o["settings"].(map[string]any)
		if !ok {
			return nil, false
		}
		ns, ok := n["settings"].(map[string]any)
		if !ok {
			return nil, false
		}
		oc, _ := os["clients"].([]any)
		nc, _ := ns["clients"].([]any)
		byEmail := map[string]any{}
		for _, v := range nc {
			u, ok := v.(map[string]any)
			if !ok {
				return nil, false
			}
			email, _ := u["email"].(string)
			if email == "" || byEmail[email] != nil {
				return nil, false
			}
			byEmail[email] = v
		}
		oldEmails := map[string]bool{}
		for _, v := range oc {
			u, ok := v.(map[string]any)
			if !ok {
				return nil, false
			}
			email, _ := u["email"].(string)
			if email == "" || oldEmails[email] || !reflect.DeepEqual(v, byEmail[email]) {
				return nil, false
			}
			oldEmails[email] = true
		}
		added := []any{}
		for _, v := range nc {
			u := v.(map[string]any)
			if !oldEmails[u["email"].(string)] {
				added = append(added, v)
			}
		}
		if len(added) > 0 {
			entry := map[string]any{}
			for k, v := range n {
				entry[k] = v
			}
			settings := map[string]any{}
			for k, v := range ns {
				settings[k] = v
			}
			settings["clients"] = added
			entry["settings"] = settings
			additions = append(additions, entry)
		}

		delete(os, "clients")
		delete(ns, "clients")
	}
	if !reflect.DeepEqual(old, next) || len(additions) == 0 {
		return nil, false
	}
	raw, e := json.Marshal(map[string]any{"inbounds": additions})
	return raw, e == nil
}
func (m *Manager) addUsersLive(ctx context.Context, raw []byte) error {
	f, e := os.CreateTemp(filepath.Dir(m.ConfigPath), ".users-*.json")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(raw); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	// The file is mode 0600; credentials never enter command arguments or logs.
	out, e := m.Runtime.Run(ctx, m.Binary, "api", "adu", "--server=127.0.0.1:10085", "--timeout=8", f.Name())
	if e != nil {
		return e
	}
	var cfg struct {
		Inbounds []struct{ Settings struct{ Clients []any } }
	}
	if e = json.Unmarshal(raw, &cfg); e != nil {
		return e
	}
	expected := 0
	for _, i := range cfg.Inbounds {
		expected += len(i.Settings.Clients)
	}
	match := regexp.MustCompile(`Added ([0-9]+) user\(s\) in total\.`).FindSubmatch(out)
	if len(match) != 2 {
		return errors.New("Xray did not acknowledge live user additions")
	}
	count, _ := strconv.Atoi(string(match[1]))
	if count != expected {
		return fmt.Errorf("Xray acknowledged %d of %d live user additions", count, expected)
	}
	return nil
}
