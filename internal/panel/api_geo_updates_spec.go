package panel

func geoUpdatesAPISpec(paths, schemas Data) {
	param := []any{Data{"name": "id", "in": "path", "required": true, "schema": Data{"type": "integer", "minimum": 1}}}
	responses := Data{"200": Data{"description": "Managed Xray geo asset capability, installed SHA-256, rollback availability and job status"}, "403": Data{"description": "Superuser administrator scope required"}, "502": Data{"description": "Agent unavailable or update rejected"}}
	schemas["GeoInstallInput"] = Data{
		"type": "object", "additionalProperties": false, "required": []any{"kind"},
		"properties": Data{
			"kind":     Data{"type": "string", "enum": []any{"geoip", "geosite"}},
			"tag":      Data{"type": "string", "maxLength": 64, "description": "Verified official stable release tag; required unless rollback is true"},
			"rollback": Data{"type": "boolean", "default": false},
		},
	}
	paths["/api/nodes/{id}/geo-manager"] = Data{"parameters": param, "get": Data{"summary": "Superuser-only installed geo assets, manual-download policy and background job status", "responses": responses}}
	paths["/api/nodes/{id}/geo-releases"] = Data{
		"parameters": append(param, Data{"name": "kind", "in": "query", "required": true, "schema": Data{"type": "string", "enum": []any{"geoip", "geosite"}}}),
		"get":        Data{"summary": "Stable official v2fly geo releases with GitHub asset SHA-256 metadata", "responses": responses},
	}
	paths["/api/nodes/{id}/geo-install"] = Data{
		"parameters": param,
		"post": Data{
			"summary":     "Queue verified geo replacement or previous-asset rollback; serialize with core changes",
			"requestBody": Data{"required": true, "content": Data{"application/json": Data{"schema": Data{"$ref": "#/components/schemas/GeoInstallInput"}}}},
			"responses":   Data{"202": Data{"description": "Accepted; poll geo-manager until success or error"}, "400": Data{"description": "Invalid kind, tag or input fields"}, "403": Data{"description": "Superuser administrator scope required"}, "502": Data{"description": "Agent unavailable, downloads disabled, operation busy or update rejected"}},
		},
	}
}
