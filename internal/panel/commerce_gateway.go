package panel

import (
	"context"
	"net/http"
)

// Provider integrations are intentionally unavailable. Invoice and financial
// history remain intact; no provider credentials or network calls are used.
func (s *Server) requestPayment(ctx context.Context, id string) (any, int, error) {
	o, e := getOrder(ctx, s.Store.Pool, id, false)
	if e != nil {
		return nil, 0, e
	}
	if str(o, "currency") == "XTR" {
		return s.requestStarsPayment(ctx, id)
	}
	store, e := getStore(ctx, s.Store.Pool, num(o, "owner_id"))
	if e != nil {
		return nil, 0, e
	}
	if !flag(store, "enabled") {
		return nil, 0, fail(409, "store is unavailable")
	}
	methods := manualMethods(store, str(o, "currency"))
	if len(methods) == 0 {
		return nil, 0, fail(409, "no payment method is enabled for this currency")
	}
	return Data{"gateway": "manual", "methods": methods}, 200, nil
}
func (s *Server) verifyPayment(ctx context.Context, id, authority string) error {
	return fail(410, "payment verification requires a Stars receipt or an administrator-reviewed transfer")
}
func (s *Server) paymentCallback(w http.ResponseWriter, r *http.Request, id string) {
	http.Error(w, "Payment gateway unavailable. Contact the seller about an existing payment.", http.StatusGone)
}
