package panel

func operationsAPISpec(paths, schemas Data) {
	integer := func(min, max int64) Data { return Data{"type": "integer", "minimum": min, "maximum": max} }
	text := Data{"type": "string"}
	boolean := Data{"type": "boolean"}
	object := func(properties Data) Data {
		return Data{"type": "object", "additionalProperties": false, "properties": properties}
	}
	body := func(name string) Data {
		return Data{"required": true, "content": Data{"application/json": Data{"schema": Data{"$ref": "#/components/schemas/" + name}}}}
	}
	responses := Data{"200": Data{"description": "Administrator result"}, "403": Data{"description": "Administrator role and scope required"}}
	param := []any{Data{"name": "id", "in": "path", "required": true, "schema": integer(1, 9223372036854775807)}}
	monitor := object(Data{"name": Data{"type": "string", "minLength": 1, "maxLength": 96}, "node": integer(1, 9223372036854775807), "core": Data{"enum": []any{"xray", "singbox"}}, "tag": Data{"type": "string", "pattern": "^[a-zA-Z0-9._-]{1,96}$"}, "enabled": boolean, "interval_seconds": integer(60, 86400), "failure_threshold": integer(1, 10), "url": Data{"type": "string", "format": "uri", "description": "Public HTTPS probe target, encrypted at rest; only detail returns full URL"}})
	schemas["MonitorInput"] = monitor
	schemas["NodeBillingInput"] = object(Data{"enabled": boolean, "provider": Data{"type": "string", "maxLength": 160}, "region": Data{"type": "string", "maxLength": 160}, "reference": Data{"type": "string", "maxLength": 160}, "currency": Data{"type": "string", "pattern": "^[A-Z]{3}$"}, "price_minor": integer(0, 1000000000000), "budget_bytes": integer(0, 9223372036854775807), "warning_percent": integer(1, 100), "reset_day": integer(1, 31), "timezone": Data{"type": "string", "description": "IANA timezone"}})
	paths["/api/monitors"] = Data{"get": Data{"summary": "List monitor metadata without target URLs", "responses": responses}, "post": Data{"summary": "Create an administrator monitor, at most 100", "requestBody": body("MonitorInput"), "responses": Data{"201": Data{"description": "Created"}, "403": Data{"description": "Administrator required"}}}}
	paths["/api/monitors/{id}"] = Data{"parameters": param, "get": Data{"summary": "Monitor detail including decrypted target", "responses": responses}, "patch": Data{"requestBody": body("MonitorInput"), "responses": responses}, "delete": Data{"responses": Data{"204": Data{"description": "Deleted together with samples"}}}}
	paths["/api/monitors/{id}/probe"] = Data{"parameters": param, "post": Data{"summary": "Run isolated node/core/tag probe without changing the production core", "responses": Data{"200": Data{"description": "Probe result, including unsuccessful target checks"}, "409": Data{"description": "Busy, removed, or configuration changed"}, "403": Data{"description": "Administrator required"}}}}
	paths["/api/monitors/{id}/history"] = Data{"parameters": param, "get": Data{"summary": "Newest 100 bounded, sanitized monitor samples", "responses": responses}}
	paths["/api/operations/nodes/{id}/billing"] = Data{"parameters": param, "get": Data{"summary": "Private hosting metadata, raw managed-traffic usage, and newest 24 monthly periods", "responses": responses}, "patch": Data{"summary": "Change private node billing policy; preserve counters and current reset boundary", "requestBody": body("NodeBillingInput"), "responses": responses}}
	paths["/api/operations/nodes/{id}/diagnostics"] = Data{"parameters": param, "get": Data{"summary": "Agent/core status and up to 50 structured deployment operation records per core", "responses": responses}}
	// This is the operational subset of the larger panel settings object.
	schemas["NotificationSettingsPatch"] = object(Data{"smtp_enabled": boolean, "smtp_host": text, "smtp_port": integer(1, 65535), "smtp_tls_mode": Data{"enum": []any{"tls", "starttls"}}, "smtp_from": text, "smtp_username": text, "smtp_password": Data{"type": "string", "maxLength": 4096, "writeOnly": true, "description": "Encrypted; omitted or blank preserves existing secret"}, "smtp_clear_password": Data{"type": "boolean", "writeOnly": true}, "alert_quota_percent": integer(0, 99), "alert_expiry_days": integer(0, 365), "alert_inactive_days": integer(0, 365)})
	obj(schemas, "NotificationSettingsPatch")["additionalProperties"] = true
	paths["/api/settings"] = Data{"get": Data{"summary": "Panel settings; SMTP secret never returned, smtp_configured is a read-only boolean", "responses": responses}, "patch": Data{"summary": "Partial panel settings update; documented schema covers notification fields, other existing settings also accepted", "requestBody": body("NotificationSettingsPatch"), "responses": responses}}
	schemas["SubscriptionEndpointInput"] = object(Data{"port": integer(1, 65535), "revision": text})
	obj(schemas, "SubscriptionEndpointInput")["required"] = []any{"port", "revision"}
	paths["/api/subscription-endpoint"] = Data{
		"get": Data{"summary": "Superuser administrator-only requested/applied subscription port, public origin and application status", "responses": responses},
		"put": Data{"summary": "Request a separate HTTPS subscription port; links move only after successful listener application", "requestBody": body("SubscriptionEndpointInput"), "responses": Data{"202": Data{"description": "Queued; poll GET until ready"}, "400": Data{"description": "Reserved port or local inbound conflict"}, "403": Data{"description": "Superuser administrator scope required"}, "409": Data{"description": "Stale revision or manual deployment"}}},
	}

}
