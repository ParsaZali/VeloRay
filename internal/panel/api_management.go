package panel

import (
	"fmt"
	"net/http"
)

func (s *Server) resetAPIKey(w http.ResponseWriter, r *http.Request, id int64, a Actor) (any, int, error) {
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	if !flag(a.User, "is_superuser") {
		return nil, 0, fail(403, "administrator access required")
	}
	ctx := r.Context()
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(842100,2)`); e != nil {
		return nil, 0, e
	}
	d, e := get(ctx, tx, "api_keys", id)
	if e != nil {
		return nil, 0, e
	}
	token := "vr_" + randomToken(32)
	d["_token_hash"] = digest(token)
	d["prefix"] = token[:15]
	d["last_used_at"] = nil
	if e = save(ctx, tx, "api_keys", d); e != nil {
		return nil, 0, e
	}
	if e = audit(ctx, tx, num(a.User, "id"), "api_keys.reset", fmt.Sprint(id), Data{}); e != nil {
		return nil, 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, 0, e
	}
	out := public(d)
	out["token"] = token
	return out, 200, nil
}
func openAPISpec(base string) Data {
	paths := Data{}
	paths["/api/clients/inbound-options"] = Data{"get": Data{"summary": "Select assigned user connections without infrastructure access", "description": "Requires clients.read. Returns only IDs, labels, protocol/core and client/batch/metering support. Resellers receive only assigned connections; no addresses, ports, secrets, TLS files, stream settings or other users' counts.", "responses": Data{"200": Data{"description": "Assigned connection choices"}, "403": Data{"description": "Client access required"}}}}
	paths["/api/clients/bulk"] = Data{"post": Data{"summary": "Atomically enable, disable, reset or delete selected users", "requestBody": Data{"required": true, "content": Data{"application/json": Data{"schema": Data{"type": "object", "additionalProperties": false, "required": []any{"ids", "action"}, "properties": Data{"ids": Data{"type": "array", "minItems": 1, "maxItems": 200, "uniqueItems": true, "items": Data{"type": "integer", "minimum": 1}}, "action": Data{"type": "string", "enum": []any{"enable", "disable", "reset", "delete"}}, "whole_accounts": Data{"type": "boolean", "default": false, "description": "Explicitly authorize deletion of every connection and shared subscription represented by a selected client. Legacy connection-only deletion still rejects shared accounts."}}}}}}, "responses": Data{"200": Data{"description": "All selected users applied on every affected node"}, "409": Data{"description": "Shared deletion requires whole_accounts or membership changed"}}}}
	for _, kind := range []string{"customers", "services", "profiles", "snippets", "nodes", "inbounds", "clients", "outbounds", "routing_rules", "admin/users", "admin/api-keys"} {
		path := "/api/" + kind
		paths[path] = Data{"get": Data{"summary": "List " + kind, "responses": Data{"200": Data{"description": "OK"}}}, "post": Data{"summary": "Create " + kind, "requestBody": Data{"required": true, "content": Data{"application/json": Data{"schema": Data{"type": "object"}}}}, "responses": Data{"201": Data{"description": "Created"}}}}
		paths[path+"/{id}"] = Data{"parameters": []any{Data{"name": "id", "in": "path", "required": true, "schema": Data{"type": "integer"}}}, "get": Data{"responses": Data{"200": Data{"description": "OK"}}}, "patch": Data{"responses": Data{"200": Data{"description": "Updated"}}}, "delete": Data{"responses": Data{"204": Data{"description": "Deleted"}}}}
	}
	for _, endpoint := range []string{"ips", "devices", "delivery-check", "subscription-history"} {
		paths["/api/clients/{id}/"+endpoint] = Data{"get": Data{"summary": "List client " + endpoint, "parameters": []any{Data{"name": "id", "in": "path", "required": true, "schema": Data{"type": "integer"}}}, "responses": Data{"200": Data{"description": "OK"}}}}
	}
	for _, kind := range []string{"profiles", "snippets"} {
		paths["/api/"+kind+"/{id}/revisions"] = Data{"get": Data{"summary": "List encrypted configuration revision metadata", "responses": Data{"200": Data{"description": "Up to 50 revisions"}}}}
		paths["/api/"+kind+"/{id}/restore"] = Data{"post": Data{"summary": "Restore a revision as a new version and deploy affected nodes", "responses": Data{"200": Data{"description": "Restored"}}}}
	}
	paths["/api/profiles/{id}/assign"] = Data{"post": Data{"summary": "Assign a shared profile to node_ids atomically", "responses": Data{"200": Data{"description": "Assignments applied"}}}}
	paths["/api/customers/bulk"] = Data{"post": Data{"summary": "Atomically patch, enable, disable, reset or delete 1–200 customers and deploy all affected nodes", "requestBody": Data{"required": true, "content": Data{"application/json": Data{"schema": Data{"type": "object", "required": []any{"ids", "action"}, "additionalProperties": false, "properties": Data{"ids": Data{"type": "array", "minItems": 1, "maxItems": 200, "uniqueItems": true, "items": Data{"type": "integer", "minimum": 1}}, "action": Data{"type": "string", "enum": []any{"patch", "enable", "disable", "reset", "delete"}}, "patch": Data{"type": "object", "description": "Quota/expiry/renewal/access fields only. Required only for patch; see customer-management.md."}}}}}}, "responses": Data{"200": Data{"description": "All selected accounts applied"}, "409": Data{"description": "Concurrent membership change; retry"}}}}
	paths["/api/customers/{id}/rotate-token"] = Data{"post": Data{"summary": "Rotate a shared subscription and revoke every historical connection token", "responses": Data{"200": Data{"description": "All subscription URLs rotated"}}}}
	paths["/api/outbound-traffic"] = Data{"get": Data{"summary": "Independent outbound telemetry by node/core, requires nodes.read and traffic.read without owner restrictions", "parameters": []any{Data{"name": "node", "in": "query", "required": true, "schema": Data{"type": "integer", "minimum": 1}}, Data{"name": "core", "in": "query", "required": true, "schema": Data{"type": "string", "enum": []any{"xray", "singbox"}}}, Data{"name": "hours", "in": "query", "schema": Data{"type": "integer", "minimum": 1, "maximum": 8760, "default": 24}}}, "responses": Data{"200": Data{"description": "Up to 1000 tags, tracked totals and retained period totals"}}}}
	paths["/api/metrics"] = Data{"get": Data{"summary": "Prometheus aggregate metrics; administrator-owned read token required", "responses": Data{"200": Data{"description": "Prometheus text exposition"}}}}
	paths["/api/subscription-template/validate"] = Data{"post": Data{"summary": "Validate a custom HTML template with sample data; administrator scope required", "responses": Data{"200": Data{"description": "Validation result"}}}}
	paths["/api/clients/{id}/device-action"] = Data{"post": Data{"summary": "Revoke, allow or forget a registered device", "responses": Data{"200": Data{"description": "Device updated"}}}}
	paths["/api/admin/api-keys/{id}/reset"] = Data{"post": Data{"parameters": []any{Data{"name": "id", "in": "path", "required": true, "schema": Data{"type": "integer"}}}, "responses": Data{"200": Data{"description": "Replacement key returned once; previous key revoked"}}}}
	schemas := featureAPISpec(paths)
	schemas["LoginInput"] = Data{"type": "object", "required": []any{"username", "password"}, "properties": Data{
		"username":  Data{"type": "string", "maxLength": 150},
		"password":  Data{"type": "string", "maxLength": 1024, "writeOnly": true},
		"provider":  Data{"type": "string", "enum": []any{"local", "ldap"}, "default": "local"},
		"totp_code": Data{"type": "string", "writeOnly": true},
	}}
	paths["/api/auth/login"] = Data{"post": Data{"security": []any{}, "summary": "Sign in with a local password or an explicitly bound LDAP directory identity", "description": "Same-origin CSRF cookie and X-CSRFToken header required. LDAP uses the panel username and directory password; local account permissions and TOTP/recovery apply. No automatic provisioning or password fallback.", "requestBody": Data{"required": true, "content": Data{"application/json": Data{"schema": Data{"$ref": "#/components/schemas/LoginInput"}}}}, "responses": Data{"200": Data{"description": "Session cookie created"}, "202": Data{"description": "Retry with the same provider, credentials and a local second-factor code"}, "401": Data{"description": "Invalid credentials or second factor"}, "429": Data{"description": "Too many attempts"}}}}
	paths["/api/auth/oidc"] = Data{"get": Data{"security": []any{}, "summary": "Available login methods", "responses": Data{"200": Data{"description": "Public booleans only; no provider credentials or LDAP address", "content": Data{"application/json": Data{"schema": Data{"type": "object", "properties": Data{"enabled": Data{"type": "boolean"}, "github_enabled": Data{"type": "boolean"}, "ldap_enabled": Data{"type": "boolean"}, "telegram": Data{"type": "boolean"}}}}}}}}}
	ldapBinding := Data{"type": "object", "description": "Administrator account fields. ldap_dn is optional; empty removes the binding and revokes sessions. Other documented account fields are accepted.", "properties": Data{"ldap_dn": Data{"type": "string", "maxLength": 1024, "description": "Exact full distinguished name bound to one account; requires configured LDAPS. Never a password."}}}
	obj(paths, "/api/admin/users")["post"].(Data)["requestBody"] = Data{"required": true, "content": Data{"application/json": Data{"schema": ldapBinding}}}
	obj(paths, "/api/admin/users/{id}")["patch"].(Data)["requestBody"] = Data{"required": true, "content": Data{"application/json": Data{"schema": ldapBinding}}}

	return Data{"openapi": "3.1.0", "info": Data{"title": "VeloRay API", "version": Version}, "servers": []any{Data{"url": base}}, "security": []any{Data{"bearerAuth": []any{}}}, "components": Data{"schemas": schemas, "securitySchemes": Data{"bearerAuth": Data{"type": "http", "scheme": "bearer"}, "cookieAuth": Data{"type": "apiKey", "in": "cookie", "name": "veloray_session"}}}, "paths": paths}
}
