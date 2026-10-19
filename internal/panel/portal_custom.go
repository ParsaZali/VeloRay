package panel

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/sys/unix"
)

// Templates are operator-owned files. os.Root confines reads even across symlinks.
// Never expose arbitrary server files through a subscriber-supplied path.
func (s *Server) templateFile(path string) (*os.Root, string, error) {
	rootPath := s.Config.TemplateRoot
	if rootPath == "" {
		rootPath = "/etc/veloray/templates"
	}
	if path == "" {
		return nil, "", errors.New("choose a template file")
	}
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return nil, "", errors.New("template must be inside the template directory")
		}
		path = rel
	}
	if !filepath.IsLocal(path) || strings.Contains(path, "\\") {
		return nil, "", errors.New("template must be inside the template directory")
	}
	if !oneOf(strings.ToLower(filepath.Ext(path)), ".html", ".htm", ".tmpl") {
		return nil, "", errors.New("template must be an HTML or TMPL file")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, "", errors.New("template directory is unavailable to the panel service")
	}
	return root, path, nil
}
func readRegular(root *os.Root, path string, limit int64) ([]byte, error) {
	// Stat before opening also rejects FIFOs and devices without blocking.
	info, err := root.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("file must be readable, regular and within the size limit")
	}
	file, err := root.OpenFile(path, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("file is not readable")
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("regular file required")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("file exceeds size limit")
	}
	return data, nil
}
func (s *Server) loadPortalTemplate(path string) (*template.Template, error) {
	root, name, err := s.templateFile(path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := readRegular(root, name, 512<<10)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(strings.ToLower(string(data)), "<html") {
		return nil, errors.New("template must contain an HTML document")
	}
	tmpl, err := template.New("custom").Option("missingkey=error").Parse(prepareLegacyPortal(string(data)))
	if err != nil {
		return nil, errors.New("template syntax is invalid; use Go HTML template syntax")
	}
	return tmpl, nil
}

type limitedTemplateWriter struct {
	io.Writer
	Remaining int
}

func (w *limitedTemplateWriter) Write(p []byte) (int, error) {
	if len(p) > w.Remaining {
		return 0, errors.New("rendered template exceeds 2 MiB")
	}
	n, err := w.Writer.Write(p)
	w.Remaining -= n
	return n, err
}
func (s *Server) portalFallback(w http.ResponseWriter, err error) {
	w.Header().Set("X-VeloRay-Template-Fallback", "true")
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	// Do not include rendered HTML, tokens or subscriber data in logs.
	logger.Warn("custom subscription template unavailable; using built-in page")
}
func (s *Server) validatePortalAPI(w http.ResponseWriter, r *http.Request, a Actor) (any, int, error) {
	if !flag(a.User, "is_superuser") || a.Scope != "admin" {
		return nil, 0, fail(403, "administrator access required")
	}
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	input, err := readBody(w, r)
	if err != nil {
		return nil, 0, err
	}
	if err = allowed(input, "path"); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	tmpl, err := s.loadPortalTemplate(str(input, "path"))
	if err != nil {
		return nil, 0, fail(400, err.Error())
	}
	d := portalSampleData()
	var out bytes.Buffer
	if err = tmpl.Execute(&limitedTemplateWriter{Writer: &out, Remaining: 2 << 20}, d); err != nil {
		return nil, 0, fail(400, "template cannot render with the supported subscription variables")
	}
	return Data{"valid": true, "bytes": out.Len()}, 200, nil
}
func portalSampleData() map[string]any {
	d := map[string]any{}
	for _, key := range strings.Fields("Locale Direction Site Name Protocol Host Active UsageLabel ExpiryLabel Used Quota Expiry URL URLLabel CopyLabel CopyDoneLabel CopyFailedLabel Connect QRLabel URILabel AppsLabel SupportLabel Link Profile Support Footer Announcement AnnouncementURL ProxyLabel ActualLabel Actual FactorLabel Factor ChartURL LastUsedISO SubscriptionTheme Accent Remaining ExpiryISO AssetsURL ThemeLabel DarkLabel LightLabel SystemLabel DetailsLabel RemainingLabel Proxy") {
		d[key] = ""
	}
	d["Locale"], d["Direction"], d["URL"], d["AssetsURL"] = "en", "ltr", "https://example.test/sub/example", "https://example.test/sub/example/assets/"
	d["Name"], d["Site"] = "Example user", "VeloRay"
	for _, key := range strings.Fields("Compact Limited ShowQR ShowApps ShowURI Clash SingBox Xray HasDeviceLimit ShowAnalytics ShowDownloads") {
		d[key] = false
	}
	for _, key := range strings.Fields("Percent ConnectionCount DeviceCount DeviceLimit") {
		d[key] = 0
	}
	d["QR"] = template.URL("")
	d["Connections"] = []Data{}
	d["Protocols"] = []string{}
	for key, value := range legacyPortalData(Data{}, defaultSettings(), d["URL"].(string), nil) {
		d[key] = value
	}
	return d
}
func (s *Server) portalAsset(w http.ResponseWriter, r *http.Request, cfg Data, path string) {
	if str(cfg, "subscription_template") != "custom" || !filepath.IsLocal(path) || strings.Contains(path, "\\") {
		http.NotFound(w, r)
		return
	}
	ext := strings.ToLower(filepath.Ext(path))
	if !oneOf(ext, ".css", ".js", ".png", ".jpg", ".jpeg", ".webp", ".gif", ".ico", ".woff", ".woff2") {
		http.NotFound(w, r)
		return
	}
	root, name, err := s.templateFile(str(cfg, "subscription_template_path"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	assets, err := root.OpenRoot(filepath.Join(filepath.Dir(name), "assets"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer assets.Close()
	data, err := readRegular(assets, path, 4<<20)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(ext))
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != "HEAD" {
		_, _ = w.Write(data)
	}
}

// Permit only the inline script bodies in this exact server-rendered document.
// Inline event handlers and third-party origins remain blocked.
func portalScriptPolicy(w http.ResponseWriter, document string) {
	tokenizer := html.NewTokenizer(strings.NewReader(document))
	sources := []string{"'self'"}
	script := false
	for {
		kind := tokenizer.Next()
		if kind == html.ErrorToken {
			break
		}
		switch kind {
		case html.StartTagToken:
			token := tokenizer.Token()
			script = token.Data == "script"
			if script {
				for _, attr := range token.Attr {
					if attr.Key == "src" {
						script = false
					}
				}
			}
		case html.TextToken:
			if script {
				sum := sha256.Sum256(tokenizer.Text())
				sources = append(sources, fmt.Sprintf("'sha256-%s'", base64.StdEncoding.EncodeToString(sum[:])))
			}
		case html.EndTagToken:
			script = false
		}
	}
	policy := "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src " + strings.Join(sources, " ") + "; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
	w.Header().Set("Content-Security-Policy", policy)
}
