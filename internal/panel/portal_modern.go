package panel

import (
	_ "embed"
	"encoding/base64"
	qrcode "github.com/skip2/go-qrcode"
	"html/template"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

//go:embed portal_modern.html
var modernPortalHTML string

var modernPortalTemplate = template.Must(template.New("modern-portal").Parse(modernPortalHTML))

// This template uses the existing subscription token and legacy settings.
// No second authorization system, raw-data evaluation or third-party CDN.
func modernPortalData(d map[string]any, c Data, cfg Data, details []Data) {
	// Read history on the origin serving the portal, including custom ports and aliases.
	address, _ := url.Parse(d["URL"].(string))
	d["ChartURL"] = address.EscapedPath() + "?format=stats"
	d["ConnectionCount"] = 0
	d["DeviceCount"] = len(obj(c, "_device_seen"))
	d["DeviceLimit"] = num(c, "device_limit")
	d["HasDeviceLimit"] = num(c, "device_limit") > 0
	d["LastUsedISO"] = ""
	if value, err := timestamp(c, "last_traffic_at"); err == nil {
		d["LastUsedISO"] = value.UTC().Format("2006-01-02T15:04:05Z")
	}
	d["ShowAnalytics"] = flag(cfg, "subscription_show_analytics")
	d["ShowDownloads"] = flag(cfg, "subscription_show_downloads")
	d["SubscriptionTheme"] = str(cfg, "subscription_theme")
	d["Accent"] = str(cfg, "accent")
	d["Remaining"] = "Unlimited"
	if limit := num(c, "traffic_limit_bytes"); limit > 0 {
		used := num(c, "used_traffic_bytes")
		if used < 0 {
			used = 0
		}
		if used >= limit {
			d["Remaining"] = "0 B"
		} else {
			d["Remaining"] = fmtBytes(limit - used)
		}
	}
	d["ExpiryISO"] = ""
	if v, err := timestamp(c, "expires_at"); err == nil {
		d["ExpiryISO"] = v.UTC().Format("2006-01-02T15:04:05Z")
	}
	filtered := []Data{}
	protocols := map[string]bool{}
	for _, item := range details {
		uri := str(item, "link")
		configProfile := str(item, "profile")
		if uri == "" && configProfile == "" {
			continue
		}
		protocol := strings.ToUpper(str(item, "protocol"))
		protocols[protocol] = true
		entry := Data{
			"Name": str(item, "name"), "Protocol": protocol,
			"Node": str(item, "node"), "Host": str(item, "host"),
			"Link": uri, "Search": strings.Join([]string{str(item, "name"), protocol, str(item, "node"), str(item, "host")}, " "),
		}
		if uri == "" {
			entry["Link"] = configProfile
			entry["ProfileURL"] = d["URL"].(string) + "?format=wireguard&connection=" + strconv.FormatInt(num(item, "index"), 10)
		}
		if str(item, "protocol") == "mtproto" {
			if parsed, err := url.Parse(uri); err == nil && parsed.Scheme == "tg" && parsed.Host == "proxy" {
				entry["OpenURL"] = "https://t.me/proxy?" + parsed.RawQuery
			}
		}
		if flag(cfg, "subscription_show_qr") {
			qrValue := uri
			if uri == "" {
				qrValue = configProfile
			}
			entry["QR"] = subscriptionQRCode(qrValue)
		}
		filtered = append(filtered, entry)
	}
	sorted := make([]string, 0, len(protocols))
	for p := range protocols {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	d["Protocols"] = sorted
	d["ConnectionCount"] = len(filtered)
	d["Connections"] = filtered
}

func subscriptionQRCode(uri string) template.URL {
	data, err := makeQRCode(uri)
	if err != nil {
		return ""
	}
	return template.URL("data:image/png;base64," + data)
}

func makeQRCode(uri string) (string, error) {
	data, err := qrcode.Encode(uri, qrcode.Medium, 240)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
