package panel

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// Bulk changes use one transaction and the same compensated deployment as an
// individual customer edit. Neither credentials nor accounting totals are editable.
func (s *Server) bulkCustomers(w http.ResponseWriter, r *http.Request, a Actor) (any, int, error) {
	if r.Method != http.MethodPost {
		return nil, 0, fail(405, "method not allowed")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(input, "ids action patch"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if e = typed(input, Data{"ids": []any{}, "action": "", "patch": Data{}}); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	action := str(input, "action")
	if !oneOf(action, "patch", "enable", "disable", "reset", "delete") {
		return nil, 0, fail(400, "choose patch, enable, disable, reset or delete")
	}
	if action == "delete" && !permitted(a.User, "clients.delete") {
		return nil, 0, fail(403, "permission required: clients.delete")
	}
	ids, e := idArray(input, "ids", 200)
	if e != nil {
		return nil, 0, e
	}
	if len(ids) == 0 || len(ids) != len(array(input, "ids")) {
		return nil, 0, fail(400, "choose 1–200 unique customer IDs")
	}
	for _, value := range array(input, "ids") {
		n, ok := value.(json.Number)
		if !ok {
			return nil, 0, fail(400, "customer IDs must be integers")
		}
		if _, e := n.Int64(); e != nil {
			return nil, 0, fail(400, "customer IDs must be integers")
		}
	}
	patch := obj(input, "patch")
	if action != "patch" && len(patch) > 0 || action == "patch" && len(patch) == 0 {
		return nil, 0, fail(400, "a nonempty patch is required only for the patch action")
	}
	if e = allowed(patch, planEditable+" expires_at next_renewal_at inbound_ids service_ids"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if e = typed(patch, customerDefaults()); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	ctx := r.Context()
	nodes := []int64{}
	// Discover both old and proposed membership before acquiring deployment locks.
	for _, id := range ids {
		d, e := get(ctx, s.Store.Pool, "customers", id)
		if e != nil {
			return nil, 0, e
		}
		if e = customerAccess(a, d); e != nil {
			return nil, 0, e
		}
		members, e := customerMembers(ctx, s.Store.Pool, id)
		if e != nil {
			return nil, 0, e
		}
		related, e := clientNodesForMembers(ctx, s.Store.Pool, members)
		if e != nil {
			return nil, 0, e
		}
		nodes = append(nodes, related...)
		if action == "patch" {
			inbounds, e := effectiveCustomerInbounds(ctx, s.Store.Pool, merge(clone(d), patch))
			if e != nil {
				return nil, 0, e
			}
			related, e := nodesForInbounds(ctx, s.Store.Pool, inbounds)
			if e != nil {
				return nil, 0, e
			}
			nodes = append(nodes, related...)
		}
	}
	nodes = nodeIDs(nodes...)
	out, e := s.managementChange(ctx, nodes, a, "customers.bulk-"+action, fmt.Sprintf("%d accounts", len(ids)), func(tx pgx.Tx) (any, error) {
		for _, id := range ids {
			d, e := get(ctx, tx, "customers", id)
			if e != nil {
				return nil, e
			}
			if e = customerAccess(a, d); e != nil {
				return nil, e
			}
			members, e := customerMembers(ctx, tx, id)
			if e != nil {
				return nil, e
			}
			related, e := clientNodesForMembers(ctx, tx, members)
			if e != nil {
				return nil, e
			}
			if e = ensureCovered(related, nodes); e != nil {
				return nil, e
			}
			if action == "delete" {
				if _, e = tx.Exec(ctx, "DELETE FROM vr_clients WHERE account_id=$1", id); e != nil {
					return nil, e
				}
				if e = remove(ctx, tx, "customers", id); e != nil {
					return nil, e
				}
				continue
			}
			prior := clone(d)
			if action == "patch" {
				d = merge(d, patch)
				reanchorRenewal(d, prior, patch)
				// Access edits do not implicitly reapply defaults from an old plan.
				if _, changed := patch["service_ids"]; changed {
					present := false
					for _, v := range array(d, "service_ids") {
						present = present || num(Data{"id": v}, "id") == num(d, "plan_id")
					}
					if !present {
						d["plan_id"] = 0
					}
				}
			} else if e = applyClientAction(d, action); e != nil {
				return nil, e
			}
			// Save the candidate before validating every connection so owner budgets
			// count the updated principal once, even when it spans multiple nodes.
			if e = save(ctx, tx, "customers", d); e != nil {
				return nil, e
			}
			if e = s.syncCustomer(ctx, tx, a, d, nodes); e != nil {
				return nil, e
			}
		}
		return Data{"count": len(ids), "status": "applied"}, nil
	})
	return out, 200, e
}
