package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

type geoEntry struct {
	Tag       string `json:"tag"`
	Reference string `json:"reference"`
	Count     int    `json:"count"`
}
type geoAsset struct {
	Kind     string     `json:"kind"`
	SHA256   string     `json:"sha256"`
	Size     int64      `json:"size"`
	Modified time.Time  `json:"modified"`
	Total    int        `json:"total"`
	Matched  int        `json:"matched"`
	Entries  []geoEntry `json:"entries"`
}
type geoCacheItem struct {
	asset   geoAsset
	checked time.Time
}
type geoCatalog struct {
	mu    sync.Mutex
	cache map[string]geoCacheItem
}

// The asset path is operator-controlled, never supplied by the API caller.
// Only the two fixed Xray protobuf files can be read. No updater or downloader
// is exposed through this endpoint.
func (g *geoCatalog) browse(ctx context.Context, kind, query string, limit int) (geoAsset, error) {
	if kind != "geoip" && kind != "geosite" {
		return geoAsset{}, errors.New("choose geoip or geosite")
	}
	if len(query) > 96 || strings.ContainsAny(query, "\r\n\x00") || limit < 1 || limit > 100 {
		return geoAsset{}, errors.New("invalid asset search or limit")
	}
	query = strings.ToLower(strings.TrimSpace(query))
	base := os.Getenv("XRAY_LOCATION_ASSET")
	if base == "" {
		base = "/usr/local/share/xray"
	}
	path := filepath.Join(base, kind+".dat")
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return geoAsset{}, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 64<<20 {
		return geoAsset{}, errors.New("installed geo asset is unavailable or exceeds 64 MiB")
	}
	cached, ok := g.cache[kind]
	if !ok || cached.asset.Size != info.Size() || !cached.asset.Modified.Equal(info.ModTime()) || time.Since(cached.checked) > 5*time.Minute {
		file, err := os.Open(path)
		if err != nil {
			return geoAsset{}, errors.New("installed geo asset is unreadable")
		}
		stat, err := file.Stat()
		if err != nil || !stat.Mode().IsRegular() || stat.Size() > 64<<20 {
			file.Close()
			return geoAsset{}, errors.New("installed geo asset is unavailable")
		}
		raw, err := io.ReadAll(io.LimitReader(file, (64<<20)+1))
		file.Close()
		if err != nil || len(raw) > 64<<20 {
			return geoAsset{}, errors.New("installed geo asset is unreadable")
		}
		entries, err := indexGeoAsset(ctx, kind, raw)
		if err != nil {
			return geoAsset{}, errors.New("installed geo asset has an invalid protobuf index")
		}
		hash := sha256.Sum256(raw)
		cached = geoCacheItem{asset: geoAsset{Kind: kind, SHA256: hex.EncodeToString(hash[:]), Size: int64(len(raw)), Modified: stat.ModTime(), Total: len(entries), Entries: entries}, checked: time.Now()}
		if g.cache == nil {
			g.cache = map[string]geoCacheItem{}
		}
		g.cache[kind] = cached
	}
	out := cached.asset
	out.Entries = []geoEntry{}
	for _, entry := range cached.asset.Entries {
		if !strings.Contains(entry.Tag, query) {
			continue
		}
		out.Matched++
		if len(out.Entries) < limit {
			out.Entries = append(out.Entries, entry)
		}
	}
	return out, nil
}

// GeoIPList and GeoSiteList both contain repeated length-delimited entries in
// field 1. Each entry has a country/tag in field 1 and CIDRs/domains in field 2.
// We inspect the envelope only, without loading individual address/domain data.
func indexGeoAsset(ctx context.Context, kind string, raw []byte) ([]geoEntry, error) {
	entries := []geoEntry{}
	seen := map[string]bool{}
	invalid := errors.New("invalid geo index")
	for len(raw) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		num, typ, n := protowire.ConsumeTag(raw)
		if n < 0 {
			return nil, invalid
		}
		raw = raw[n:]
		if num != 1 {
			n = protowire.ConsumeFieldValue(num, typ, raw)
			if n < 0 {
				return nil, invalid
			}
			raw = raw[n:]
			continue
		}
		if typ != protowire.BytesType || len(entries) >= 20000 {
			return nil, invalid
		}
		body, n := protowire.ConsumeBytes(raw)
		if n < 0 {
			return nil, invalid
		}
		raw = raw[n:]
		tag, count := "", 0
		for len(body) > 0 {
			num, typ, n = protowire.ConsumeTag(body)
			if n < 0 {
				return nil, invalid
			}
			body = body[n:]
			if num == 1 || num == 2 {
				if typ != protowire.BytesType {
					return nil, invalid
				}
				value, size := protowire.ConsumeBytes(body)
				if size < 0 {
					return nil, invalid
				}
				body = body[size:]
				if num == 1 {
					if tag != "" {
						return nil, invalid
					}
					tag = strings.ToLower(string(value))
				} else {
					count++
				}
			} else {
				n = protowire.ConsumeFieldValue(num, typ, body)
				if n < 0 {
					return nil, invalid
				}
				body = body[n:]
			}
		}
		if len(tag) == 0 || len(tag) > 128 || seen[tag] {
			return nil, invalid
		}
		for _, c := range tag {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.ContainsRune("-_.!", c)) {
				return nil, invalid
			}
		}
		seen[tag] = true
		entries = append(entries, geoEntry{Tag: tag, Reference: kind + ":" + tag, Count: count})
	}
	if len(entries) == 0 {
		return nil, invalid
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Tag < entries[j].Tag })
	return entries, nil
}
