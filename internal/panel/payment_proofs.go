package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const proofSelect = `SELECT jsonb_build_object('id',p.id,'order_id',p.order_id,'method',p.method,'state',p.state,'network',p.network,'transaction_id',p.transaction_id,'created_at',p.created_at,'reviewed_at',p.reviewed_at,'review_note',p.review_note,'_evidence',p.evidence_ciphertext) FROM vr_payment_proofs p `

func (s *Server) paymentProof(ctx context.Context, q Query, id string, evidence bool) (Data, error) {
	var raw []byte
	e := q.QueryRow(ctx, proofSelect+`WHERE p.order_id=$1`, id).Scan(&raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	d, e := decode(raw)
	if e != nil {
		return nil, e
	}
	if evidence {
		plain, err := unseal(s.Config.FieldKey, str(d, "_evidence"))
		if err != nil {
			return nil, err
		}
		v, err := decode([]byte(plain))
		if err != nil {
			return nil, err
		}
		d["evidence"] = v
	}
	return public(d), nil
}
func validateProofImage(value string) error {
	if value == "" {
		return nil
	}
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 || !oneOf(parts[0], "data:image/jpeg;base64", "data:image/png;base64", "data:image/webp;base64") {
		return fail(400, "use a JPG, PNG or WebP receipt")
	}
	b, e := base64.StdEncoding.DecodeString(parts[1])
	if e != nil || len(b) == 0 || len(b) > 1<<20 {
		return fail(400, "receipt image must be at most 1 MiB")
	}
	mime := http.DetectContentType(b)
	expected := strings.TrimSuffix(strings.TrimPrefix(parts[0], "data:"), ";base64")
	if mime != expected {
		return fail(400, "receipt content does not match its image type")
	}
	return nil
}
func (s *Server) submitPaymentProof(w http.ResponseWriter, r *http.Request, o Data) (any, int, error) {
	if r.Method == "GET" {
		d, e := s.paymentProof(r.Context(), s.Store.Pool, str(o, "id"), false)
		return d, 200, e
	}
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	if e := rateLimit(r.Context(), s.Store.Pool, "proof:"+str(o, "id"), 8, time.Hour); e != nil {
		return nil, 0, e
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(d, "method transaction_id image note"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if e = typed(d, Data{"method": "", "transaction_id": "", "image": "", "note": ""}); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	out, e := s.recordPaymentProof(r.Context(), str(o, "id"), d)
	return out, 201, e
}
func (s *Server) recordPaymentProof(ctx context.Context, id string, d Data) (Data, error) {
	if len(str(d, "note")) > 1200 {
		return nil, fail(400, "receipt note is too long")
	}
	if e := validateProofImage(str(d, "image")); e != nil {
		return nil, e
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	o, e := getOrder(ctx, tx, id, true)
	if e != nil {
		return nil, e
	}
	expiry, _ := timestamp(o, "expires_at")
	if str(o, "state") != "pending" || !expiry.After(time.Now()) {
		return nil, fail(409, "order is not awaiting payment or has expired")
	}
	if str(o, "gateway") != "" && !strings.HasPrefix(str(o, "gateway"), "manual-") {
		return nil, fail(409, "order uses another payment method")
	}
	store, e := getStore(ctx, tx, num(o, "owner_id"))
	if e != nil {
		return nil, e
	}
	if !flag(store, "enabled") {
		return nil, fail(409, "store is unavailable")
	}
	var method Data
	for _, v := range manualMethods(store, str(o, "currency")) {
		m := asData(v)
		if str(m, "method") == str(d, "method") {
			method = m
		}
	}
	if method == nil {
		return nil, fail(400, "payment method is unavailable for this order")
	}
	network, transaction := "", strings.TrimSpace(str(d, "transaction_id"))
	var txValue any
	if str(d, "method") == "usdt" {
		if !transactionHash.MatchString(transaction) {
			return nil, fail(400, "provide the 64-character transaction hash")
		}
		network = str(method, "network")
		transaction = strings.ToLower(strings.TrimPrefix(transaction, "0x"))
		txValue = transaction
	} else if str(d, "image") == "" {
		return nil, fail(400, "upload the bank transfer receipt")
	}
	// Pin destination details to the review, even if the seller edits its settings.
	evidence := clone(d)
	evidence["destination"] = method
	raw, _ := json.Marshal(evidence)
	sealed, e := seal(s.Config.FieldKey, string(raw))
	if e != nil {
		return nil, e
	}
	var oldState string
	e = tx.QueryRow(ctx, `SELECT state FROM vr_payment_proofs WHERE order_id=$1 FOR UPDATE`, id).Scan(&oldState)
	if e == nil && oldState != "rejected" {
		return nil, fail(409, "a receipt for this order is already awaiting review or approved")
	}
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return nil, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO vr_payment_proofs(order_id,method,network,transaction_id,evidence_ciphertext) VALUES($1,$2,$3,$4,$5) ON CONFLICT(order_id) DO UPDATE SET method=excluded.method,network=excluded.network,transaction_id=excluded.transaction_id,evidence_ciphertext=excluded.evidence_ciphertext,state='pending',created_at=now(),reviewed_at=NULL,reviewer_id=NULL,review_note=''`, id, str(d, "method"), network, txValue, sealed)
	if e != nil {
		return nil, fail(409, "this transaction has already been submitted")
	}
	_, e = tx.Exec(ctx, `UPDATE vr_orders SET gateway=$2,expires_at=GREATEST(expires_at,now()+interval '24 hours') WHERE id=$1`, id, "manual-"+str(d, "method"))
	if e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, 0, "commerce.receipt_submitted", id, Data{"method": str(d, "method")}); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return s.paymentProof(ctx, s.Store.Pool, id, false)
}
func (s *Server) receiptsAPI(w http.ResponseWriter, r *http.Request, path string, a Actor, owner int64) (any, int, error) {
	ctx := r.Context()
	parts := strings.Split(path, "/")
	if len(parts) == 1 && r.Method == "GET" {
		rows, e := commerceRows(ctx, s.Store.Pool, proofSelect+`JOIN vr_orders o ON o.id=p.order_id WHERE o.owner_id=$1 ORDER BY p.created_at DESC LIMIT 200`, owner)
		for i, d := range rows {
			rows[i] = public(d)
		}
		return rows, 200, e
	}
	if len(parts) < 2 || len(parts) > 3 {
		return nil, 0, fail(404, "receipt not found")
	}
	id, e := strconv.ParseInt(parts[1], 10, 64)
	if e != nil {
		return nil, 0, fail(404, "receipt not found")
	}
	var order string
	if e = s.Store.Pool.QueryRow(ctx, `SELECT p.order_id FROM vr_payment_proofs p JOIN vr_orders o ON o.id=p.order_id WHERE p.id=$1 AND o.owner_id=$2`, id, owner).Scan(&order); e != nil {
		return nil, 0, fail(404, "receipt not found")
	}
	if len(parts) == 2 && r.Method == "GET" {
		out, e := s.paymentProof(ctx, s.Store.Pool, order, true)
		return out, 200, e
	}
	if len(parts) != 3 || r.Method != "POST" || !oneOf(parts[2], "approve", "reject") || !permitted(a.User, "commerce.write") {
		return nil, 0, fail(403, "receipt review permission required")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(input, "note reference expected_amount expected_currency"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if len(str(input, "note")) > 500 || parts[2] == "reject" && strings.TrimSpace(str(input, "note")) == "" {
		return nil, 0, fail(400, "provide a rejection reason of at most 500 characters")
	}
	out, e := s.managementChange(ctx, nil, a, "commerce.receipt_"+parts[2], order, func(tx pgx.Tx) (any, error) {
		o, e := getOrder(ctx, tx, order, true)
		if e != nil {
			return nil, e
		}
		if num(o, "owner_id") != owner {
			return nil, fail(404, "receipt not found")
		}
		var state, method, network string
		var hash *string
		if e = tx.QueryRow(ctx, `SELECT state,method,network,transaction_id FROM vr_payment_proofs WHERE id=$1 FOR UPDATE`, id).Scan(&state, &method, &network, &hash); e != nil {
			return nil, e
		}
		if state == "approved" && parts[2] == "approve" {
			return s.paymentProof(ctx, tx, order, false)
		}
		if state != "pending" || str(o, "state") != "pending" {
			return nil, fail(409, "receipt is no longer awaiting review")
		}
		if parts[2] == "approve" {
			if num(input, "expected_amount") != num(o, "amount") || str(input, "expected_currency") != str(o, "currency") {
				return nil, fail(409, "confirm the exact invoice amount and currency")
			}
			ref := strings.TrimSpace(str(input, "reference"))
			if method == "usdt" && hash != nil {
				ref = network + ":" + *hash
			}
			if len(ref) < 4 || len(ref) > 160 {
				return nil, fail(400, "provide the bank transfer reference")
			}
			if e = s.recheckOrderCapacity(ctx, tx, o); e != nil {
				return nil, e
			}
			_, e = tx.Exec(ctx, `UPDATE vr_orders SET state='paid',paid_at=now(),payment_reference=$2,last_error='',next_attempt_at=now() WHERE id=$1`, order, ref)
			if e != nil {
				return nil, fail(409, "payment reference was already used")
			}
			state = "approved"
		} else {
			state = "rejected"
		}
		_, e = tx.Exec(ctx, `UPDATE vr_payment_proofs SET state=$2,reviewed_at=now(),reviewer_id=$3,review_note=$4 WHERE id=$1`, id, state, num(a.User, "id"), str(input, "note"))
		if e != nil {
			return nil, e
		}
		return s.paymentProof(ctx, tx, order, false)
	})
	if e == nil && parts[2] == "approve" {
		_ = s.fulfillOrder(ctx, order)
	}
	return out, 200, e
}
func proofBotDescription(p Data) string {
	return fmt.Sprintf("#%d · %s · %s\n%s", num(p, "id"), str(p, "method"), str(p, "state"), str(p, "order_id"))
}
