package panel

import (
	"context"
	"encoding/csv"
	"net/http"
	"strconv"
	"time"
)

// Stats are available only through the same unguessable subscription token.
// Reads never register devices or change traffic. The key is a billing
// principal, not an owner/reseller ID or token.
func (s *Server) subscriptionStats(w http.ResponseWriter, r *http.Request, client Data) {
	duration, bucket, ok := subscriptionRange(r.URL.Query().Get("range"))
	if !ok {
		writeJSON(w, 400, Data{"detail": "unsupported chart range"})
		return
	}
	export := r.URL.Query().Get("export")
	if export != "" && export != "csv" {
		writeJSON(w, 400, Data{"detail": "unsupported stats export"})
		return
	}
	if r.Method == http.MethodHead {
		if export == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.WriteHeader(http.StatusOK)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	rows, err := s.Store.Pool.Query(ctx,
		`SELECT date_bin($2::interval,at,'2000-01-01 UTC'::timestamptz) AS bucket,
          COALESCE(sum(bytes),0)::bigint,
          COALESCE(sum(upload_bytes),0)::bigint,
          COALESCE(sum(download_bytes),0)::bigint
    FROM vr_client_traffic_windows
    WHERE principal_key=$1 AND at>=date_bin(interval '5 minutes',now()-$3::interval,'2000-01-01 UTC'::timestamptz)
    GROUP BY bucket ORDER BY bucket ASC LIMIT 1000`,
		accountKey(client), bucket, duration)
	if err != nil {
		s.respond(w, r, nil, 0, err)
		return
	}
	defer rows.Close()
	result := []Data{}
	for rows.Next() {
		var at time.Time
		var total, upload, download int64
		if err = rows.Scan(&at, &total, &upload, &download); err != nil {
			s.respond(w, r, nil, 0, err)
			return
		}
		result = append(result, Data{"at": at.UTC().Format(time.RFC3339), "bytes": total, "upload_bytes": upload, "download_bytes": download})
	}
	if err = rows.Err(); err != nil {
		s.respond(w, r, nil, 0, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if export == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="veloray-traffic.csv"`)
		writer := csv.NewWriter(w)
		if err := writer.Write([]string{"timestamp_utc", "total_bytes", "upload_bytes", "download_bytes"}); err != nil {
			return
		}
		for _, row := range result {
			if err := writer.Write([]string{str(row, "at"), strconv.FormatInt(num(row, "bytes"), 10), strconv.FormatInt(num(row, "upload_bytes"), 10), strconv.FormatInt(num(row, "download_bytes"), 10)}); err != nil {
				return
			}
		}
		writer.Flush()
		return
	}
	writeJSON(w, 200, Data{"range": r.URL.Query().Get("range"), "points": result, "history_available": len(result) > 0, "recorded_usage": recordedSubscriptionUsage(client)})
}

func recordedSubscriptionUsage(client Data) Data {
	up, down := num(client, "raw_upload_traffic_bytes"), num(client, "raw_download_traffic_bytes")
	raw := num(trafficDefaults(clone(client)), "raw_used_traffic_bytes")
	return Data{"bytes": raw, "upload_bytes": up, "download_bytes": down}
}

// Values are fixed and never concatenated into SQL from request parameters.
func subscriptionRange(v string) (duration string, bucket string, ok bool) {
	switch v {
	case "1h":
		return "1 hour", "5 minutes", true
	case "12h":
		return "12 hours", "30 minutes", true
	case "24h":
		return "24 hours", "1 hour", true
	case "", "7d":
		return "7 days", "6 hours", true
	case "30d":
		return "30 days", "1 day", true
	case "90d":
		return "90 days", "1 day", true
	}
	return "", "", false
}
