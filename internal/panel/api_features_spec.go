package panel

// Feature routes are documented alongside the principal CRUD schema. Secret
// fields are write-only, except the administrator-only policy body/detail URL.
func featureAPISpec(paths Data) Data {
	ok := func(description string) Data {
		return Data{"200": Data{"description": description}, "403": Data{"description": "Insufficient permission"}}
	}
	object := func(properties Data) Data {
		return Data{"type": "object", "additionalProperties": false, "properties": properties}
	}
	text := Data{"type": "string"}
	ids := Data{"type": "array", "maxItems": 200, "uniqueItems": true, "items": Data{"type": "integer", "minimum": 1}}
	policy := object(Data{"name": Data{"type": "string", "maxLength": 96}, "enabled": Data{"type": "boolean"}, "customer_ids": ids, "service_ids": ids, "body": object(Data{"url": Data{"type": "string", "description": "Public HTTPS feed; empty disables remote import"}, "links": Data{"type": "array", "maxItems": 200, "items": text}, "apps": Data{"type": "array", "items": text, "description": "Optional client presentation selectors, never authorization"}, "default_format": Data{"enum": []any{"auto", "raw", "base64", "clash", "singbox", "xray"}}, "templates": object(Data{"clash": text, "singbox": text, "xray": text})})})
	endpoint := object(Data{"name": text, "enabled": Data{"type": "boolean"}, "channel": Data{"enum": []any{"webhook", "discord", "email"}}, "url": Data{"type": "string", "description": "Public HTTPS URL, or 1–10 comma-separated recipients for email. Encrypted; administrator detail only"}, "signing_secret": Data{"type": "string", "writeOnly": true, "description": "32–1024 bytes for generic webhooks. Blank update preserves current key."}, "events": Data{"type": "array", "maxItems": 64, "items": text, "description": "Exact audit action, prefix such as client.*, or *"}})
	body := func(schema Data) Data {
		return Data{"required": true, "content": Data{"application/json": Data{"schema": schema}}}
	}
	param := []any{Data{"name": "id", "in": "path", "required": true, "schema": Data{"type": "integer", "minimum": 1}}}
	for _, entry := range []struct {
		path, summary string
		schema        Data
	}{{"/api/subscription-policies", "Subscription policies (administrator only)", policy}, {"/api/webhooks/endpoints", "Notification destinations (administrator only)", endpoint}} {
		paths[entry.path] = Data{"get": Data{"summary": entry.summary + ": metadata without secrets", "responses": ok("Metadata list")}, "post": Data{"summary": entry.summary + ": create", "requestBody": body(entry.schema), "responses": Data{"201": Data{"description": "Created"}}}}
		paths[entry.path+"/{id}"] = Data{"parameters": param, "get": Data{"summary": entry.summary + ": authorized detail", "responses": ok("Detail")}, "patch": Data{"requestBody": body(entry.schema), "responses": ok("Updated; omitted top-level fields preserved, supplied body replaces the policy body")}, "delete": Data{"responses": Data{"204": Data{"description": "Deleted; destination queue cascades"}}}}
	}
	paths["/api/subscription-policies/{id}/refresh"] = Data{"parameters": param, "post": Data{"summary": "Fetch at most 1 MiB of portable URIs into encrypted cache; retain previous cache on failure", "responses": ok("Updated cache metadata")}}
	for _, action := range []string{"test", "retry"} {
		responses := ok("Exhausted destination deliveries scheduled for retry")
		if action == "test" {
			responses = Data{"202": Data{"description": "Destination test event queued"}}
		}
		paths["/api/webhooks/endpoints/{id}/"+action] = Data{"parameters": param, "post": Data{"summary": "Administrator destination " + action, "responses": responses}}
	}
	paths["/api/events"] = Data{"get": Data{"summary": "Owner-scoped dashboard SSE; dashboard.read, 2 streams per user / 32 per process", "security": []any{Data{"cookieAuth": []any{}}, Data{"bearerAuth": []any{}}}, "responses": Data{"200": Data{"description": "overview every 10s; reauthenticate; reconnect after 2min", "content": Data{"text/event-stream": Data{"schema": text}}}, "429": Data{"description": "Stream cap reached"}}}}
	paths["/api/auth/oidc"] = Data{"get": Data{"summary": "Public SSO availability flag", "security": []any{}, "responses": ok("enabled boolean")}}
	paths["/api/auth/oidc/start"] = Data{"post": Data{"summary": "Public, CSRF-protected OIDC authorization-code start with PKCE, nonce and browser-bound state", "security": []any{}, "responses": ok("authorization_url")}}
	paths["/api/auth/oidc/callback"] = Data{"get": Data{"summary": "Provider callback; consumes state and verifies ID token before exact issuer/subject lookup", "security": []any{}, "parameters": []any{Data{"name": "state", "in": "query", "required": true, "schema": text}, Data{"name": "code", "in": "query", "required": true, "schema": text}}, "responses": Data{"303": Data{"description": "Redirect to local login, second-factor challenge or panel"}}}}
	paths["/api/auth/oidc/complete"] = Data{"post": Data{"summary": "CSRF-protected, browser challenge-bound local second-factor completion", "security": []any{}, "requestBody": body(object(Data{"totp_code": text})), "responses": ok("Local session after TOTP/recovery code validation")}}
	schemas := Data{"SubscriptionPolicyInput": policy, "WebhookDestinationInput": endpoint}
	operationsAPISpec(paths, schemas)
	networkPolicyAPISpec(paths, schemas)
	subscriptionTemplateAPISpec(paths, schemas)
	return schemas
}
