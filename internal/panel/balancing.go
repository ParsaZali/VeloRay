package panel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Xray selectors match tag prefixes, including future outbounds with that prefix.
func validateXrayRouting(cfg Data) error {
	outs := map[string]bool{}
	for _, raw := range array(cfg, "outbounds") {
		outs[str(asData(raw), "tag")] = true
	}
	groups := map[string]bool{}
	route := obj(cfg, "routing")
	if raw, ok := route["balancers"]; ok {
		if _, ok := raw.([]any); !ok {
			return fmt.Errorf("balancers must be an array")
		}
	}
	if len(array(route, "balancers")) > 64 {
		return fmt.Errorf("at most 64 balancers are supported")
	}
	for _, raw := range array(route, "balancers") {
		b := asData(raw)
		if err := allowed(b, "tag selector fallbackTag strategy"); err != nil {
			return err
		}
		if value, exists := b["fallbackTag"]; exists {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("balancer fallbackTag must be a string")
			}
		}
		if value, exists := b["strategy"]; exists {
			if _, ok := value.(map[string]any); !ok {
				if _, ok := value.(Data); !ok {
					return fmt.Errorf("balancer strategy must be an object")
				}
			}
		}
		if err := allowed(obj(b, "strategy"), "type settings"); err != nil {
			return err
		}
		tag := str(b, "tag")
		if !tagPattern.MatchString(tag) || tag == "api" || groups[tag] {
			return fmt.Errorf("invalid or duplicate balancer tag %q", tag)
		}
		groups[tag] = true
		selectors := array(b, "selector")
		if len(selectors) == 0 || len(selectors) > 64 {
			return fmt.Errorf("balancer %q requires 1–64 selector prefixes", tag)
		}
		seen := map[string]bool{}
		for _, raw := range selectors {
			prefix, ok := raw.(string)
			if !ok || !tagPattern.MatchString(prefix) || seen[prefix] {
				return fmt.Errorf("balancer %q requires unique nonempty selector prefixes", tag)
			}
			seen[prefix] = true
			matched := false
			for out := range outs {
				if strings.HasPrefix(out, prefix) {
					matched = true
				}
			}
			if !matched {
				return fmt.Errorf("balancer %q selector %q matches no enabled outbound", tag, prefix)
			}
		}
		if fallback := str(b, "fallbackTag"); fallback != "" && !outs[fallback] {
			return fmt.Errorf("balancer %q references missing fallback %q", tag, fallback)
		}
		strategy := strings.ToLower(str(obj(b, "strategy"), "type"))
		if value, exists := obj(b, "strategy")["type"]; exists {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("balancing strategy type must be a string")
			}
		}
		if !oneOf(strategy, "", "random", "roundrobin", "leastping", "leastload") {
			return fmt.Errorf("unsupported balancing strategy %q", strategy)
		}
		if oneOf(strategy, "leastping", "leastload") && len(obj(cfg, "observatory")) == 0 && len(obj(cfg, "burstObservatory")) == 0 {
			return fmt.Errorf("balancer %q requires a configured observatory", tag)
		}
	}
	for _, raw := range array(route, "rules") {
		rule := asData(raw)
		tag, group := str(rule, "outboundTag"), str(rule, "balancerTag")
		if tag != "" && group != "" {
			return fmt.Errorf("routing rule must select one outbound or balancer")
		}
		if tag != "" && tag != "api" && !outs[tag] {
			return fmt.Errorf("routing outbound %q does not exist", tag)
		}
		if group != "" && !groups[group] {
			return fmt.Errorf("routing balancer %q does not exist", group)
		}
	}
	return nil
}

func balancingView(cfg Data) Data {
	tags := []string{}
	for _, raw := range array(cfg, "outbounds") {
		if tag := str(asData(raw), "tag"); tag != "" && tag != "api" {
			tags = append(tags, tag)
		}
	}
	sort.Strings(tags)
	balancers := array(obj(cfg, "routing"), "balancers")
	if balancers == nil {
		balancers = []any{}
	}
	v := Data{"balancers": balancers, "outbound_tags": tags, "observatory_configured": len(obj(cfg, "observatory")) > 0 || len(obj(cfg, "burstObservatory")) > 0}
	raw, _ := json.Marshal(v)
	hash := sha256.Sum256(raw)
	v["revision"] = hex.EncodeToString(hash[:])
	return v
}

func (s *Server) balancingAPI(w http.ResponseWriter, r *http.Request, id int64, a Actor) (any, int, error) {
	if !flag(a.User, "is_superuser") || !oneOf(a.Scope, "admin", "read") || r.Method != "GET" && a.Scope != "admin" {
		return nil, 0, fail(403, "administrator access required")
	}
	if !oneOf(r.Method, "GET", "PUT") {
		return nil, 0, fail(405, "method not allowed")
	}
	ctx := r.Context()
	view := func(q Query, n Data) (Data, error) {
		cfg, err := s.buildConfig(ctx, q, n)
		if err != nil {
			return nil, err
		}
		return balancingView(cfg), nil
	}
	if r.Method == "GET" {
		n, err := get(ctx, s.Store.Pool, "nodes", id)
		if err != nil {
			return nil, 0, err
		}
		out, err := view(s.Store.Pool, n)
		return out, 200, err
	}
	input, err := readBody(w, r)
	if err != nil {
		return nil, 0, err
	}
	if err = allowed(input, "balancers revision"); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	if _, ok := input["balancers"].([]any); !ok || len(array(input, "balancers")) > 64 {
		return nil, 0, fail(400, "balancers must be an array of at most 64 entries")
	}
	if len(str(input, "revision")) != 64 {
		return nil, 0, fail(400, "current balancing revision required")
	}
	raw, _ := json.Marshal(input)
	if len(raw) > 65536 {
		return nil, 0, fail(400, "balancing configuration exceeds 64 KiB")
	}
	out, err := s.change(ctx, nodeIDs(id), a, "node.balancing", strconv.FormatInt(id, 10), func(tx pgx.Tx) (any, error) {
		n, err := get(ctx, tx, "nodes", id)
		if err != nil {
			return nil, err
		}
		current, err := view(tx, n)
		if err != nil {
			return nil, err
		}
		if str(current, "revision") != str(input, "revision") {
			return nil, fail(409, "balancing configuration changed; reload before saving")
		}
		patch := clone(obj(n, "config_patch"))
		route := obj(patch, "routing")
		route["balancers"] = input["balancers"]
		patch["routing"] = route
		n["config_patch"] = patch
		if err = save(ctx, tx, "nodes", n); err != nil {
			return nil, err
		}
		return view(tx, n)
	})
	return out, 200, err
}
