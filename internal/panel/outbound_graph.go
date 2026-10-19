package panel

import "fmt"

// Validate references after profiles and managed outbounds have been merged.
// Runtime checks alone do not consistently detect recursive dialer chains.
func validateOutboundGraph(cfg Data, core string) error {
	entries := map[string]Data{}
	for _, raw := range array(cfg, "outbounds") {
		d := obj(Data{"item": raw}, "item")
		tag := str(d, "tag")
		if tag == "" {
			continue
		} // An untagged default cannot be referenced.
		if entries[tag] != nil {
			return fmt.Errorf("duplicate outbound tag %q", tag)
		}
		entries[tag] = d
	}
	edges := map[string][]string{}
	for tag, d := range entries {
		refs := []string{}
		if core == "xray" {
			refs = append(refs, str(obj(d, "proxySettings"), "tag"), str(obj(obj(d, "streamSettings"), "sockopt"), "dialerProxy"))
		} else {
			if str(d, "type") == "direct" && str(d, "detour") != "" {
				return fmt.Errorf("direct outbound %q cannot have a detour in sing-box", tag)
			}
			refs = append(refs, str(d, "detour"))
			if oneOf(str(d, "type"), "selector", "urltest") {
				candidates := array(d, "outbounds")
				if len(candidates) == 0 {
					return fmt.Errorf("outbound %q requires candidates", tag)
				}
				seen := map[string]bool{}
				for _, raw := range candidates {
					candidate, ok := raw.(string)
					if !ok || candidate == "" || seen[candidate] {
						return fmt.Errorf("outbound %q requires unique, nonempty candidate tags", tag)
					}
					seen[candidate] = true
					refs = append(refs, candidate)
				}
				if def := str(d, "default"); str(d, "type") == "selector" && def != "" && !seen[def] {
					return fmt.Errorf("outbound %q default must be a candidate", tag)
				}
			}
		}
		for _, ref := range refs {
			if ref == "" {
				continue
			}
			if entries[ref] == nil {
				return fmt.Errorf("outbound %q references missing next hop %q", tag, ref)
			}
			edges[tag] = append(edges[tag], ref)
		}
	}
	state := map[string]uint8{}
	var visit func(string) error
	visit = func(tag string) error {
		if state[tag] == 1 {
			return fmt.Errorf("outbound chain contains a cycle at %q", tag)
		}
		if state[tag] == 2 {
			return nil
		}
		state[tag] = 1
		for _, ref := range edges[tag] {
			if e := visit(ref); e != nil {
				return e
			}
		}
		state[tag] = 2
		return nil
	}
	for tag := range entries {
		if e := visit(tag); e != nil {
			return e
		}
	}
	return nil
}
