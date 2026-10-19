package panel

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// The lower-camel names are the 3x-ui subscription-page contract. Directional
// values are real traffic; usedByte remains the authoritative charged quota.
func legacyPortalData(c, cfg Data, subscriptionURL string, details []Data) Data {
	used, limit := num(c, "used_traffic_bytes"), num(c, "traffic_limit_bytes")
	up, down := num(c, "raw_upload_traffic_bytes"), num(c, "raw_download_traffic_bytes")
	expiry, last := int64(0), int64(0)
	if t, e := timestamp(c, "expires_at"); e == nil {
		expiry = t.UnixMilli()
	}
	if t, e := timestamp(c, "last_traffic_at"); e == nil {
		last = t.UnixMilli()
	}
	links, emails := []string{}, []string{}
	for _, d := range details {
		if link := str(d, "link"); link != "" {
			links = append(links, link)
			emails = append(emails, str(c, "name"))
		}
	}
	total, remaining := "Unlimited", "Unlimited"
	if limit > 0 {
		total = fmtBytes(limit)
		remaining = fmtBytes(max(0, limit-used))
	}
	id := "client-" + strconv.FormatInt(num(c, "id"), 10)
	if num(c, "_account") > 0 {
		id = "account-" + strconv.FormatInt(num(c, "_account"), 10)
	}
	var online any
	if value, exists := c["_subscription_online"]; exists {
		online = value
	}
	return Data{"sId": id, "enabled": active(c, time.Now()), "isOnline": online, "lastOnline": last, "expire": expiry, "expireUnix": expiry / 1000, "downloadByte": down, "uploadByte": up, "totalByte": limit, "usedByte": used, "rawUsedByte": num(c, "raw_used_traffic_bytes"), "download": fmtBytes(down), "upload": fmtBytes(up), "used": fmtBytes(used), "total": total, "remained": remaining, "subUrl": subscriptionURL, "subJsonUrl": subscriptionURL + "?format=xray", "subClashUrl": subscriptionURL + "?format=clash", "subTitle": str(cfg, "site_name"), "subSupportUrl": str(cfg, "support_url"), "announce": str(cfg, "announcement"), "datepicker": "gregorian", "links": links, "emails": emails}
}

var legacyVariable = regexp.MustCompile(`\{\{\s*\.(subUrl|subTitle|sId|downloadByte|links)\b`)
var scriptBody = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
var clickAttribute = regexp.MustCompile(`(?i)([\s<])onclick\s*=`)
var externalScript = regexp.MustCompile(`(?is)<script\b[^>]*\bsrc=["']https://(?:unpkg\.com/@phosphor-icons/web[^"']*|cdnjs\.cloudflare\.com/ajax/libs/qrious/[^"']*|cdn\.jsdelivr\.net/npm/chart\.js[^"']*)["'][^>]*>\s*</script\s*>`)
var remoteStyle = regexp.MustCompile(`(?is)<link\b[^>]*\bhref=["']https://(?:fonts\.googleapis\.com/[^"']*|cdnjs\.cloudflare\.com/ajax/libs/font-awesome/[^"']*)["'][^>]*>`)
var remoteFont = regexp.MustCompile(`https://raw\.githubusercontent\.com/rastikerdar/vazirmatn/[^'"\s)]+`)

func prepareLegacyPortal(source string) string {
	if !legacyVariable.MatchString(source) {
		return source
	}
	// A JavaScript string containing a literal opening delimiter is not a Go
	// action. Preserve its runtime value while making it invisible to Go's lexer.
	source = scriptBody.ReplaceAllStringFunc(source, func(script string) string {
		script = strings.ReplaceAll(script, `"{{"`, `"\u007b\u007b"`)
		script = strings.ReplaceAll(script, `'{{'`, `'\u007b\u007b'`)
		return script
	})
	source = externalScript.ReplaceAllString(source, "")
	source = remoteStyle.ReplaceAllString(source, "")
	source = remoteFont.ReplaceAllString(source, "/fonts/Vazirmatn-Variable.woff2")
	source = strings.ReplaceAll(source, `document.getElementById("rawConfigsSource").innerHTML`, `document.getElementById("rawConfigsSource").textContent`)
	// Declarative handlers are dispatched by a small allowlist, never eval or
	// unsafe-inline. This also covers handler attributes in JS HTML strings.
	source = clickAttribute.ReplaceAllString(source, "${1}data-vr-click=")
	// These legacy HTML builders decode URI labels before innerHTML. Escape
	// their text/attribute substitutions; the event bridge never evaluates JS.
	for _, variable := range []string{"name", "cleanLink", "cfg", "url"} {
		source = strings.ReplaceAll(source, "${"+variable+"}", "${window.VeloRayTemplate.escapeHTML("+variable+")}")
	}
	source = strings.ReplaceAll(source, "https://api.qrserver.com/v1/create-qr-code/?size=280x280&margin=1&data=", "?format=qr&data=")
	// Supplied templates use both timestamp units; the contract is milliseconds.
	if strings.Contains(source, "function renderExpire(seconds)") {
		source = strings.ReplaceAll(source, "var ms = exp * 1000;", "var ms = exp < 1e12 ? exp * 1000 : exp;")
	}
	source = strings.ReplaceAll(source, "withFormat(D.sub, 'json')", "withFormat(D.sub, 'xray')")
	// Keep authoritative charged quota distinct from actual upload/download.
	source = strings.ReplaceAll(source, "const templateVars = {", "const templateVars = { usedByte: {{.usedByte}},")
	source = strings.ReplaceAll(source, "let usedBytes = (isNaN(uploadByteNum) ? 0 : uploadByteNum) + (isNaN(downloadByteNum) ? 0 : downloadByteNum);", "let usedBytes = Number(templateVars.usedByte);")
	source = strings.ReplaceAll(source, "var used = dl + ul;", "var used = Number(document.body.dataset.vrUsed);")
	source = strings.ReplaceAll(source, "var used = Math.max(0, dl + ul);", "var used = Number(document.body.dataset.vrUsed);")
	source = strings.ReplaceAll(source, "}).then(function (info) {", "}).then(function (info) { document.body.dataset.vrUsed = String(info.usedByte);")
	// Data is rendered by html/template in an attribute context, never trusted JS.
	bodyTag := regexp.MustCompile(`(?i)<body\b`)
	source = bodyTag.ReplaceAllString(source, `<body data-vr-used="{{.usedByte}}"`)
	assets := `<link rel="stylesheet" href="/subscription-template-compat.css"><script src="/subscription-template-compat.js"></script>`
	headEnd := regexp.MustCompile(`(?i)</head\s*>`)
	return headEnd.ReplaceAllString(source, assets+"</head>")
}

func (s *Server) subscriptionQR(w http.ResponseWriter, r *http.Request, c, cfg Data, links []string) {
	if !flag(cfg, "subscription_show_qr") {
		http.NotFound(w, r)
		return
	}
	target := r.URL.Query().Get("data")
	base := strings.TrimRight(s.Config.PublicURL, "/") + "/sub/" + str(c, "subscription_token")
	allowed := target == base
	if u, e := url.Parse(target); e == nil && strings.HasPrefix(target, base+"?") {
		q := u.Query()
		allowed = len(q) == 1 && oneOf(q.Get("format"), "base64", "raw", "xray", "singbox", "clash", "json")
	}
	connectionTarget := false
	if !allowed {
		for _, link := range links {
			if target == link {
				connectionTarget = true
				allowed = true
				break
			}
		}
	}
	if !allowed || len(target) > 8192 {
		writeJSON(w, 400, Data{"detail": "QR target does not belong to this subscription"})
		return
	}
	if connectionTarget {
		if e := s.admitSubscriptionDevice(w, r, c, cfg); e != nil {
			s.respond(w, r, nil, 0, e)
			return
		}
	}
	png, e := qrcode.Encode(target, qrcode.Medium, 280)
	if e != nil {
		writeJSON(w, 400, Data{"detail": "QR data is too long"})
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != "HEAD" {
		_, _ = w.Write(png)
	}
}

func legacySubscriptionInfo(c, cfg Data) Data {
	values := legacyPortalData(c, cfg, "", nil)
	out := Data{}
	for _, key := range strings.Fields("enabled isOnline lastOnline expire expireUnix downloadByte uploadByte totalByte usedByte rawUsedByte download upload used total remained") {
		out[key] = values[key]
	}
	return out
}

// The independent subscription service exposes only public presentation assets,
// never the administrator SPA or API. Both modes share these exact files.
func (s *Server) serveSubscriptionAsset(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	allowed := oneOf(path, "/subscription-portal.js", "/subscription-modern.js", "/subscription-modern.css", "/subscription-simple.js", "/subscription-simple.css", "/subscription-template-compat.js", "/subscription-template-compat.css")
	if strings.HasPrefix(path, "/fonts/") || strings.HasPrefix(path, "/subscription-template-assets/") {
		allowed = oneOf(strings.ToLower(filepath.Ext(path)), ".woff2", ".woff", ".css") && filepath.Clean(path) == path
	}
	if !allowed {
		return false
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return true
	}
	file := filepath.Join(s.Config.FrontendDir, filepath.Clean("/"+path))
	info, e := os.Stat(file)
	if e != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return true
	}
	http.ServeFile(w, r, file)
	return true
}
