package panel

import (
	_ "embed"
	"html/template"
)

//go:embed portal_simple.html
var simplePortalHTML string
var simplePortalTemplate = template.Must(template.New("simple-portal").Parse(simplePortalHTML))
