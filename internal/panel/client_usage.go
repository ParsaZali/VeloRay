package panel

import (
	"context"
	"fmt"
	"time"
)

// Accounting values are integers in bytes. A null remaining quota means
// unlimited, not zero. Telemetry freshness is reported, never invented.
func (s *Server) clientUsage(ctx context.Context, id int64) (Data, error) {
	c, e := get(ctx, s.Store.Pool, "clients", id)
	if e != nil {
		return nil, e
	}
	v, e := s.view(ctx, s.Store.Pool, "clients", c)
	if e != nil {
		return nil, e
	}
	out := Data{"client_id": id, "account_id": num(c, "_account"), "used_bytes": num(c, "used_traffic_bytes"), "limit_bytes": num(c, "traffic_limit_bytes"), "remaining_bytes": nil, "unlimited": num(c, "traffic_limit_bytes") == 0, "raw_used_bytes": num(c, "raw_used_traffic_bytes"), "raw_upload_bytes": num(c, "raw_upload_traffic_bytes"), "raw_download_bytes": num(c, "raw_download_traffic_bytes"), "raw_lifetime_bytes": num(c, "raw_lifetime_traffic_bytes"), "multiplier": effectiveTrafficMultiplier(c), "expires_at": c["expires_at"], "enabled": flag(c, "enabled"), "disabled_reason": str(c, "disabled_reason"), "usage_metered": v["usage_metered"], "online": v["online"], "connection_status": v["connection_status"], "telemetry_at": v["telemetry_at"], "observed_at": stamp(), "telemetry_fresh": false, "account_shared": num(c, "_account") > 0}
	if num(c, "traffic_limit_bytes") > 0 {
		out["remaining_bytes"] = max(int64(0), num(c, "traffic_limit_bytes")-num(c, "used_traffic_bytes"))
	}
	if at, e := timestamp(v, "telemetry_at"); e == nil {
		age := time.Since(at)
		out["telemetry_fresh"] = age >= 0 && age <= 20*time.Second
	}
	for _, key := range []string{"used_bytes", "limit_bytes", "raw_used_bytes", "raw_upload_bytes", "raw_download_bytes", "raw_lifetime_bytes"} {
		out[key+"_exact"] = fmt.Sprint(out[key])
	}
	if out["remaining_bytes"] != nil {
		out["remaining_bytes_exact"] = fmt.Sprint(out["remaining_bytes"])
	} else {
		out["remaining_bytes_exact"] = nil
	}
	return out, nil
}
