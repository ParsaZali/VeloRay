package panel

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"
)

// Negative values are discounts: -0.5 bills 50%, -0.25 bills 75%.
// Thousandths and a persistent remainder keep small accounting polls exact.
func multiplierMilli(d Data) (*big.Int, error) {
	v, exists := d["traffic_multiplier"]
	if !exists {
		return big.NewInt(1000), nil
	}
	r, ok := new(big.Rat).SetString(fmt.Sprint(v))
	if !ok {
		return nil, errors.New("invalid traffic multiplier")
	}
	finite, _ := r.Float64()
	if math.IsInf(finite, 0) || math.IsNaN(finite) {
		return nil, errors.New("traffic multiplier must be finite")
	}
	r.Mul(r, big.NewRat(1000, 1))
	if !r.IsInt() {
		return nil, errors.New("traffic multiplier supports at most three decimal places")
	}
	n := new(big.Int).Set(r.Num())
	if n.Sign() < 0 {
		n.Add(n, big.NewInt(1000))
		if n.Sign() < 0 {
			n.SetInt64(0)
		}
	}
	return n, nil
}

func trafficDefaults(c Data) Data {
	if _, ok := c["traffic_multiplier"]; !ok {
		c["traffic_multiplier"] = 1.0
	}
	if _, ok := c["raw_used_traffic_bytes"]; !ok {
		c["raw_used_traffic_bytes"] = num(c, "used_traffic_bytes")
	}
	if _, ok := c["raw_lifetime_traffic_bytes"]; !ok {
		c["raw_lifetime_traffic_bytes"] = num(c, "lifetime_traffic_bytes")
	}
	return c
}

func applyTrafficDelta(c Data, delta int64) error {
	trafficDefaults(c)
	rate, err := multiplierMilli(c)
	if err != nil || delta < 0 {
		return errors.New("invalid traffic multiplier or counter")
	}
	rem := num(c, "_charge_remainder")
	if rem < 0 || rem >= 1000 {
		return errors.New("invalid traffic remainder")
	}
	value := new(big.Int).Mul(big.NewInt(delta), rate)
	value.Add(value, big.NewInt(rem))
	charged, remainder := new(big.Int), new(big.Int)
	charged.QuoRem(value, big.NewInt(1000), remainder)
	if !charged.IsInt64() {
		return errors.New("client traffic overflow")
	}
	for key, add := range map[string]int64{
		"used_traffic_bytes": charged.Int64(), "lifetime_traffic_bytes": charged.Int64(),
		"raw_used_traffic_bytes": delta, "raw_lifetime_traffic_bytes": delta,
	} {
		old := num(c, key)
		if old < 0 || add > int64(^uint64(0)>>1)-old {
			return errors.New("client traffic overflow")
		}
	}
	for _, key := range []string{"used_traffic_bytes", "lifetime_traffic_bytes"} {
		c[key] = num(c, key) + charged.Int64()
	}
	for _, key := range []string{"raw_used_traffic_bytes", "raw_lifetime_traffic_bytes"} {
		c[key] = num(c, key) + delta
	}
	c["_charge_remainder"] = remainder.Int64()
	c["last_traffic_at"] = stamp()
	activateFirstUse(c, time.Now(), delta > 0)
	return nil
}

func applyDirectionalTrafficDelta(c Data, direction string, delta int64) error {
	key := "raw_upload_traffic_bytes"
	if direction == "downlink" {
		key = "raw_download_traffic_bytes"
	} else if direction != "uplink" {
		return errors.New("invalid traffic direction")
	}
	old := num(c, key)
	if delta < 0 || old < 0 || delta > int64(^uint64(0)>>1)-old {
		return errors.New("directional traffic overflow")
	}
	if err := applyTrafficDelta(c, delta); err != nil {
		return err
	}
	c[key] = old + delta
	return nil
}

func effectiveTrafficMultiplier(c Data) float64 {
	rate, err := multiplierMilli(c)
	if err != nil {
		return 0
	}
	value, _ := new(big.Rat).SetFrac(rate, big.NewInt(1000)).Float64()
	return value
}

var accountFields = []string{"_renewal_clock", "renewal_period", "renewal_every", "renewal_timezone", "_renewal_day", "hwid_limit", "device_limit", "start_after_first_use", "duration_days", "first_used_at", "_device_seen", "_device_denied", "owner_id", "enabled", "expires_at", "traffic_limit_bytes", "used_traffic_bytes", "lifetime_traffic_bytes", "raw_used_traffic_bytes", "raw_lifetime_traffic_bytes", "raw_upload_traffic_bytes", "raw_download_traffic_bytes", "traffic_multiplier", "_charge_remainder", "last_traffic_at", "disabled_reason", "renewal_interval_days", "next_renewal_at"}

func accountKey(c Data) string {
	if id := num(c, "_account"); id > 0 {
		return "account:" + strconv.FormatInt(id, 10)
	}
	return "client:" + strconv.FormatInt(num(c, "id"), 10)
}

func resetTraffic(c Data) {
	c["raw_upload_traffic_bytes"], c["raw_download_traffic_bytes"] = int64(0), int64(0)
	c["used_traffic_bytes"], c["raw_used_traffic_bytes"], c["_charge_remainder"] = int64(0), int64(0), int64(0)
}
