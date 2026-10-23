package panel

func expansionAPISpec(paths, schemas Data) {
	text := Data{"type": "string"}
	integer := Data{"type": "integer"}
	currency := Data{"type": "string", "enum": []any{"IRR", "IRT", "USD", "EUR"}, "description": "Whole rials/tomans; USD/EUR amounts are cents. No currency conversion between wallets."}
	object := func(fields Data) Data {
		return Data{"type": "object", "additionalProperties": false, "properties": fields}
	}
	body := func(name string) Data {
		return Data{"required": true, "content": Data{"application/json": Data{"schema": Data{"$ref": "#/components/schemas/" + name}}}}
	}
	response := Data{"200": Data{"description": "Completed"}, "400": Data{"description": "Invalid input"}, "403": Data{"description": "Permission required"}, "404": Data{"description": "Not found in your scope"}, "409": Data{"description": "Conflict; refresh or inspect the order"}}
	owner := []any{Data{"name": "owner", "in": "query", "schema": Data{"type": "integer", "minimum": 1}, "description": "Omit for self; only a superuser admin can select another seller."}}
	schemas["OfferInput"] = object(Data{"name": Data{"type": "string", "maxLength": 96}, "description": Data{"type": "string", "maxLength": 1200}, "enabled": Data{"type": "boolean"}, "service_id": integer, "amount": Data{"type": "integer", "minimum": 1, "maximum": moneyLimit}, "currency": currency, "duration_days": Data{"type": "integer", "minimum": 1, "maximum": 3650}, "traffic_bytes": Data{"type": "integer", "minimum": 0, "description": "Zero means unlimited. Renewals add finite traffic without resetting consumed usage."}, "cost_amount": Data{"type": "integer", "minimum": 0, "maximum": moneyLimit}})
	schemas["LedgerAdjustment"] = object(Data{"amount": Data{"type": "integer", "minimum": -moneyLimit, "maximum": moneyLimit, "description": "Nonzero signed amount; append-only correction, never rewrite a ledger row."}, "currency": currency, "reference": Data{"type": "string", "minLength": 12, "maxLength": 160, "description": "Unique idempotency reference"}, "note": Data{"type": "string", "minLength": 1, "maxLength": 500}})
	schemas["StorefrontInput"] = object(Data{"enabled": Data{"type": "boolean"}, "name": text, "description": text, "logo_url": Data{"type": "string", "description": "HTTPS URL or empty"}, "support_url": Data{"type": "string", "description": "HTTPS URL or empty"}})
	schemas["WalletPurchaseInput"] = object(Data{"offer_id": integer, "kind": Data{"enum": []any{"purchase", "renewal", "traffic"}}, "name": text, "customer_token": Data{"type": "string", "writeOnly": true, "description": "Canonical subscription token required for renewal/traffic"}, "reference": Data{"type": "string", "minLength": 12, "maxLength": 160, "description": "Retry with the same reference and identical body to prevent duplicate debits"}})
	schemas["PaymentGatewayInput"] = object(Data{"payments_enabled": Data{"type": "boolean"}, "merchant_id": Data{"type": "string", "writeOnly": true}, "sandbox": Data{"type": "boolean", "description": "Test payments never credit wallets or provision customers"}})
	paths["/api/commerce/offers"] = Data{"parameters": owner, "get": Data{"summary": "Seller-scoped offer list", "responses": response}, "post": Data{"summary": "Create draft or published sale offer", "requestBody": body("OfferInput"), "responses": Data{"201": Data{"description": "Created"}}}}
	paths["/api/commerce/offers/{id}"] = Data{"parameters": append(append([]any{}, owner...), Data{"name": "id", "in": "path", "required": true, "schema": integer}), "patch": Data{"summary": "Edit offer; existing invoices retain their immutable price and grant", "requestBody": body("OfferInput"), "responses": response}}
	paths["/api/commerce/store"] = Data{"parameters": owner, "get": Data{"summary": "Own storefront branding", "responses": response}, "patch": Data{"summary": "Publish/store branding", "requestBody": body("StorefrontInput"), "responses": response}}
	for _, entry := range []struct{ path, summary string }{{"wallets", "Balances and credit limits by currency"}, {"ledger", "Newest 500 append-only entries"}, {"orders", "Newest 500 seller or payer orders; private payment fields excluded"}, {"reports", "Revenue, cost and profit by currency over 1–3660 days; top-ups excluded"}, {"costs", "Operating cost entries"}} {
		paths["/api/commerce/"+entry.path] = Data{"parameters": owner, "get": Data{"summary": entry.summary, "responses": response}}
	}
	paths["/api/commerce/adjust"] = Data{"parameters": owner, "post": Data{"summary": "Superuser-only signed wallet adjustment/settlement", "requestBody": body("LedgerAdjustment"), "responses": response}}
	paths["/api/commerce/purchase"] = Data{"post": Data{"summary": "Debit own wallet and safely provision/renew; paid orders retry after deployment failure", "requestBody": body("WalletPurchaseInput"), "responses": Data{"201": Data{"description": "Order created or replayed; inspect its state"}}}}
	paths["/api/commerce/settings"] = Data{"get": Data{"summary": "Superuser gateway availability, without merchant secret", "responses": response}, "patch": Data{"summary": "Configure verified Zarinpal gateway", "requestBody": body("PaymentGatewayInput"), "responses": response}}
	paths["/api/commerce/orders/{id}/fulfill"] = Data{"parameters": []any{Data{"name": "id", "in": "path", "required": true, "schema": text}}, "post": Data{"summary": "Idempotent retry of an owned paid order; never marks an unpaid order as paid", "responses": response}}
	paths["/api/shop/catalog"] = Data{"get": Data{"summary": "Public published offers for the store query parameter; no unit costs or inbound IDs", "security": []any{}, "parameters": []any{Data{"name": "store", "in": "query", "required": true, "schema": integer}}, "responses": response}}
	paths["/api/shop/orders"] = Data{"post": Data{"summary": "Create public invoice; origin/CSRF protected and rate limited. Returns order token once.", "security": []any{}, "requestBody": body("WalletPurchaseInput"), "responses": Data{"201": Data{"description": "Order ID, amount, currency and private bearer token"}}}}
	orderParams := []any{Data{"name": "id", "in": "path", "required": true, "schema": text}, Data{"name": "Authorization", "in": "header", "required": true, "schema": text, "description": "Bearer ORDER_TOKEN, not a panel API key"}}
	paths["/api/shop/orders/{id}"] = Data{"parameters": orderParams, "get": Data{"summary": "Private order status and delivered subscription after fulfillment", "security": []any{}, "responses": response}}
	for _, action := range []string{"pay", "verify"} {
		paths["/api/shop/orders/{id}/"+action] = Data{"parameters": orderParams, "post": Data{"summary": "Order-token protected provider " + action + "; amount/authority come from the persisted invoice", "security": []any{}, "responses": response}}
	}
	for _, action := range []string{"freeze", "resume"} {
		paths["/api/customers/{id}/"+action] = Data{"parameters": []any{Data{"name": "id", "in": "path", "required": true, "schema": integer}}, "post": Data{"summary": "Owner-scoped bounded subscription " + action + "; maximum 30 days per annual allowance, stable token and preserved usage", "requestBody": Data{"content": Data{"application/json": Data{"schema": object(Data{"days": Data{"type": "integer", "minimum": 1, "maximum": 30}})}}}, "responses": response}}
	}
	paths["/api/services/{id}/impact"] = Data{"parameters": []any{Data{"name": "id", "in": "path", "required": true, "schema": integer}}, "post": Data{"summary": "Preview service membership changes without deploying; send returned revision as expected_revision on PATCH", "responses": response}}
	paths["/api/node-enrollment"] = Data{"get": Data{"summary": "Administrator enrollment metadata without tokens", "responses": response}, "post": Data{"summary": "Create a 15-minute single-use node installation command", "requestBody": Data{"content": Data{"application/json": Data{"schema": object(Data{"name": text, "public_host": text})}}}, "responses": Data{"201": Data{"description": "Token and installation command, shown once"}}}}
	paths["/api/node-enrollment/{id}"] = Data{"parameters": []any{Data{"name": "id", "in": "path", "required": true, "schema": text}}, "delete": Data{"summary": "Revoke unused enrollment token", "responses": response}}
	paths["/api/node-enrollment/complete"] = Data{"post": Data{"summary": "Single-use enrollment bearer; verifies host, pinned certificate, agent credentials, identity and running core before registration", "security": []any{}, "responses": Data{"201": Data{"description": "Registered node ID"}, "404": Data{"description": "Invalid, expired, revoked or spent token"}, "502": Data{"description": "Agent verification failed; token stays unused"}}}}
}
