package panel

func salesExperienceSpec(paths, schemas Data) {
	text := Data{"type": "string"}
	integer := Data{"type": "integer", "format": "int64"}
	boolean := Data{"type": "boolean"}
	response := Data{"200": Data{"description": "Success"}, "400": Data{"description": "Invalid input"}, "401": Data{"description": "Authentication required"}, "403": Data{"description": "Scope/permission required"}, "404": Data{"description": "Not found in your ownership scope"}, "409": Data{"description": "State conflict; reload before retrying"}}
	object := func(props Data) Data {
		return Data{"type": "object", "additionalProperties": false, "properties": props}
	}
	body := func(props Data) Data {
		return Data{"required": true, "content": Data{"application/json": Data{"schema": object(props)}}}
	}
	id := []any{Data{"name": "id", "in": "path", "required": true, "schema": integer}}
	owner := Data{"name": "owner", "in": "query", "schema": integer, "description": "Authorized seller; omit for self"}
	schemas["ClientUsage"] = object(Data{"client_id": integer, "account_id": integer, "used_bytes": integer, "limit_bytes": integer, "remaining_bytes": Data{"type": []any{"integer", "null"}}, "unlimited": boolean, "raw_upload_bytes": integer, "raw_download_bytes": integer, "raw_used_bytes": integer, "used_bytes_exact": text, "remaining_bytes_exact": Data{"type": []any{"string", "null"}}, "telemetry_at": Data{"type": []any{"string", "null"}}, "telemetry_fresh": boolean, "account_shared": boolean, "online": Data{"type": []any{"boolean", "null"}}, "usage_metered": boolean})
	paths["/api/deployment"] = Data{"get": Data{"summary": "Administrator-only deployment metadata; does not expose Docker daemon access or secrets", "responses": response}}
	paths["/api/clients/{id}/usage"] = Data{"parameters": id, "get": Data{"summary": "Owner-scoped exact-byte accounting, remaining quota and telemetry freshness; null remaining means unlimited; shared account billing must not be counted per connection", "responses": response}}
	botProps := Data{}
	for k, v := range salesDefaults() {
		switch v.(type) {
		case bool:
			botProps[k] = boolean
		case int, int64:
			botProps[k] = integer
		case string:
			botProps[k] = text
		case []any:
			botProps[k] = Data{"type": "array", "maxItems": 24, "items": Data{"type": "object"}}
		}
	}
	schemas["SalesBotSettings"] = object(botProps)
	if props := obj(obj(schemas, "StorefrontInput"), "properties"); len(props) > 0 {
		props["bot"] = Data{"$ref": "#/components/schemas/SalesBotSettings"}
	}
	if props := obj(obj(schemas, "OfferInput"), "properties"); len(props) > 0 {
		props["category"] = Data{"type": "string", "maxLength": 96}
		props["image_url"] = Data{"type": "string", "description": "HTTPS image URL"}
		props["sort_order"] = Data{"type": "integer", "minimum": 0, "maximum": 10000}
	}
	delete(schemas, "PaymentGatewayInput")
	for _, resource := range []string{"receipts", "tickets"} {
		paths["/api/commerce/"+resource] = Data{"parameters": []any{owner}, "get": Data{"summary": "Authorized seller " + resource + "; private evidence is excluded from lists", "responses": response}}
		paths["/api/commerce/"+resource+"/{id}"] = Data{"parameters": append(append([]any{}, id...), owner), "get": Data{"summary": "Authorized private " + resource + " detail", "responses": response}}
	}
	for _, action := range []string{"approve", "reject"} {
		paths["/api/commerce/receipts/{id}/"+action] = Data{"parameters": append(append([]any{}, id...), owner), "post": Data{"summary": "Manual transfer review; approval requires independent verification and exact invoice amount/currency; idempotent delivery", "requestBody": body(Data{"expected_amount": integer, "expected_currency": text, "reference": text, "note": Data{"type": "string", "maxLength": 500}}), "responses": response}}
	}
	paths["/api/commerce/tickets/{id}"].(Data)["post"] = Data{"summary": "Reply to or close an owned support ticket", "requestBody": body(Data{"text": Data{"type": "string", "maxLength": 2000}, "close": boolean}), "responses": response}
	miniParams := []any{Data{"name": "store", "in": "query", "required": true, "schema": integer}, Data{"name": "X-Telegram-Init-Data", "in": "header", "required": true, "schema": Data{"type": "string", "description": "Raw Telegram-signed initData; not a panel API key"}}}
	for _, path := range []string{"/api/shop/me", "/api/shop/me/orders", "/api/shop/me/tickets"} {
		paths[path] = Data{"parameters": miniParams, "get": Data{"summary": "Telegram-verified buyer's own data; enabled seller store required", "security": []any{}, "responses": response}}
	}
	paths["/api/shop/me/trial"] = Data{"parameters": miniParams, "post": Data{"summary": "Once-per-buyer trial; policy, membership, existing purchase and seller quota enforced", "security": []any{}, "requestBody": body(Data{"accept_terms": boolean}), "responses": response}}
	for _, action := range []string{"enable", "disable", "rotate"} {
		paths["/api/shop/me/services/{id}/"+action] = Data{"parameters": append(append([]any{}, miniParams...), id...), "post": Data{"summary": "Seller-enabled action for the signed buyer's own service; explicit confirmation required", "security": []any{}, "requestBody": body(Data{"confirm": boolean}), "responses": response}}
	}
	paths["/api/shop/me/tickets"].(Data)["post"] = Data{"summary": "Open a private support ticket", "security": []any{}, "requestBody": body(Data{"subject": text, "text": text}), "responses": response}
	paths["/api/shop/me/tickets/{id}"] = Data{"parameters": append(append([]any{}, miniParams...), id...), "get": Data{"summary": "Own ticket messages", "security": []any{}, "responses": response}, "post": Data{"summary": "Reply to or close own ticket", "security": []any{}, "requestBody": body(Data{"text": text, "close": boolean}), "responses": response}}
	orderParams := []any{Data{"name": "id", "in": "path", "required": true, "schema": text}, Data{"name": "Authorization", "in": "header", "required": true, "schema": text, "description": "Bearer ORDER_TOKEN, not a panel key"}}
	paths["/api/shop/orders/{id}/receipt"] = Data{"parameters": orderParams, "post": Data{"summary": "Submit image/transaction evidence; never marks a payment paid; JPG/PNG/WebP up to 1 MiB, USDT hash unique by network", "security": []any{}, "requestBody": body(Data{"method": Data{"type": "string", "enum": []any{"card", "usdt"}}, "image": Data{"type": "string", "writeOnly": true}, "transaction_id": text, "note": Data{"type": "string", "maxLength": 1200}}), "responses": response}, "get": Data{"summary": "Order-token-protected receipt status without evidence", "security": []any{}, "responses": response}}
}
