import { useEffect, useState } from "react";
import { RefreshCw, Check, MessageSquare, ShieldCheck } from "lucide-react";
import { api } from "@/lib/api";
import { amountLabel, commerceError, type Order } from "@/lib/commerce";
import type { PaymentProof, Ticket, TicketMessage } from "@/lib/sales";
import { t } from "@/lib/i18n";
import { notify } from "@/lib/notifications";
import { Button } from "@/components/ui/button";
import { Input, Textarea } from "@/components/ui/controls";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Dialog } from "@/components/common/dialog";
export function SalesReview({
  owner,
  editable,
  orders,
  mode,
  onReviewed,
}: {
  owner: number;
  editable: boolean;
  orders: Order[];
  mode: "receipts" | "tickets";
  onReviewed: () => void;
}) {
  const [rows, setRows] = useState<(PaymentProof | Ticket)[]>([]),
    [selected, setSelected] = useState<PaymentProof | Ticket | null>(null),
    [proof, setProof] = useState<PaymentProof | null>(null),
    [messages, setMessages] = useState<TicketMessage[]>([]),
    [note, setNote] = useState(""),
    [reference, setReference] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(false),
    [revision, setRevision] = useState(0),
    [decision, setDecision] = useState<"approve" | "reject" | null>(null);
  const base = `/commerce/${mode}`,
    scope = `?owner=${owner}`;
  useEffect(() => {
    setSelected(null);
    setDecision(null);
  }, [owner, mode]);
  useEffect(() => {
    const c = new AbortController();
    setLoading(true);
    setError("");
    void api<(PaymentProof | Ticket)[]>(base + scope, { signal: c.signal })
      .then(setRows)
      .catch((e) => {
        if (!c.signal.aborted) setError(commerceError(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [base, scope, revision]);
  useEffect(() => {
    if (!selected) return;
    const c = new AbortController();
    setProof(null);
    setMessages([]);
    setNote("");
    setReference("");
    setDecision(null);
    setLoading(true);
    void api<PaymentProof | TicketMessage[]>(`${base}/${selected.id}${scope}`, {
      signal: c.signal,
    })
      .then((d) => {
        if (Array.isArray(d)) setMessages(d);
        else setProof(d);
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(commerceError(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [base, scope, selected]);
  const order = proof ? orders.find((o) => o.id === proof.order_id) : undefined;
  async function submit(action: "approve" | "reject" | "reply" | "close") {
    if (!selected || busy) return;
    setBusy(true);
    setError("");
    try {
      const path = `${base}/${selected.id}${action === "approve" || action === "reject" ? `/${action}` : ""}${scope}`;
      await api(path, {
        method: "POST",
        body: JSON.stringify(
          mode === "receipts"
            ? {
                note,
                reference,
                expected_amount: order?.amount,
                expected_currency: order?.currency,
              }
            : { text: note, close: action === "close" },
        ),
      });
      notify(t("Saved", "ذخیره شد"), "success");
      setSelected(null);
      setRevision((v) => v + 1);
      onReviewed();
    } catch (e) {
      setError(commerceError(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-4">
      {error && (
        <p role="alert" className="text-[12px] text-red-500">
          {error}
        </p>
      )}
      <div className="flex justify-end">
        <Button
          variant="outline"
          disabled={busy || loading}
          onClick={() => setRevision((v) => v + 1)}
        >
          <RefreshCw className="h-4 w-4" />
          {t("Refresh", "بروزرسانی")}
        </Button>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>
            {mode === "receipts"
              ? t("Transfer receipts", "رسیدهای پرداخت")
              : t("Support tickets", "تیکت‌های پشتیبانی")}
          </CardTitle>
          {mode === "receipts" ? (
            <ShieldCheck className="h-5 w-5 text-emerald-500" />
          ) : (
            <MessageSquare className="h-5 w-5 text-emerald-500" />
          )}
        </CardHeader>
        <CardContent className="space-y-2">
          {loading && !selected ? (
            <p role="status">{t("Loading…")}</p>
          ) : !rows.length ? (
            <p className="py-8 text-center text-[13px] text-[var(--muted)]">
              {t("No items yet.", "هنوز موردی ثبت نشده.")}
            </p>
          ) : (
            rows.map((row) => (
              <button
                key={row.id}
                className="flex w-full items-center justify-between gap-4 rounded-lg border border-[var(--border)] p-4 text-start hover:border-emerald-500/50"
                onClick={() => setSelected(row)}
              >
                <span className="min-w-0 truncate text-[13px]">
                  #{row.id} ·{" "}
                  {"subject" in row
                    ? row.subject
                    : row.method === "card"
                      ? t("Bank transfer", "کارت‌به‌کارت")
                      : `USDT · ${row.network}`}
                </span>
                <span
                  className={`shrink-0 rounded-full px-2 py-1 text-[11px] ${["approved", "closed"].includes(row.state) ? "bg-emerald-500/10 text-emerald-500" : "bg-amber-500/10 text-amber-500"}`}
                >
                  {{
                    pending: t("Awaiting review", "در انتظار بررسی"),
                    approved: t("Approved", "تأییدشده"),
                    rejected: t("Rejected", "ردشده"),
                    open: t("Open", "باز"),
                    closed: t("Closed", "بسته"),
                  }[row.state as "pending"] || row.state}
                </span>
              </button>
            ))
          )}
        </CardContent>
      </Card>
      {selected && (
        <Dialog
          label={
            mode === "receipts"
              ? t("Review receipt", "بررسی رسید")
              : t("Ticket", "تیکت")
          }
          size="lg"
          onClose={() => !busy && setSelected(null)}
        >
          <div className="space-y-5">
            <h3>
              {mode === "receipts"
                ? t("Review receipt", "بررسی رسید")
                : t("Ticket", "تیکت")}{" "}
              #{selected.id}
            </h3>
            {error && (
              <p role="alert" className="text-red-500">
                {error}
              </p>
            )}
            {loading ? (
              <p role="status">{t("Loading…")}</p>
            ) : mode === "receipts" && proof ? (
              <>
                <dl className="grid grid-cols-2 gap-4 rounded-lg border border-[var(--border)] p-4 text-[12px]">
                  <div>
                    <dt className="text-[var(--muted)]">
                      {t("Amount", "مبلغ")}
                    </dt>
                    <dd className="mt-1 font-semibold">
                      {order ? amountLabel(order.amount, order.currency) : "—"}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-[var(--muted)]">
                      {t("Order", "سفارش")}
                    </dt>
                    <dd className="mt-1 break-all font-mono text-[11px]">
                      {proof.order_id}
                    </dd>
                  </div>
                </dl>
                {proof.transaction_id && (
                  <label className="label">
                    {t("Transaction hash", "هش تراکنش")}
                    <Input readOnly dir="ltr" value={proof.transaction_id} />
                  </label>
                )}
                {proof.evidence?.destination && (
                  <p
                    className="break-all rounded-lg bg-[var(--bg)] p-3 font-mono text-[12px]"
                    dir="ltr"
                  >
                    {proof.evidence.destination.card_number ||
                      `${proof.evidence.destination.network} · ${proof.evidence.destination.address}`}
                  </p>
                )}
                {proof.evidence?.image && (
                  <a
                    href={proof.evidence.image}
                    download={`receipt-${proof.id}.png`}
                  >
                    <img
                      alt={t("Transfer receipt", "رسید پرداخت")}
                      src={proof.evidence.image}
                      className="mx-auto max-h-96 max-w-full rounded-lg object-contain"
                    />
                  </a>
                )}
                {proof.evidence?.note && (
                  <p className="whitespace-pre-wrap text-[13px]">
                    {proof.evidence.note}
                  </p>
                )}
                {proof.review_note && (
                  <p className="text-[12px] text-[var(--muted)]">
                    {proof.review_note}
                  </p>
                )}
                {editable && proof.state === "pending" && (
                  <>
                    <label className="label">
                      {t("Review note", "یادداشت بررسی")}
                      <Textarea
                        maxLength={500}
                        value={note}
                        onChange={(e) => setNote(e.target.value)}
                      />
                    </label>
                    {proof.method === "card" && (
                      <label className="label">
                        {t(
                          "Verified bank reference",
                          "شماره پیگیری تأییدشده بانک",
                        )}
                        <Input
                          maxLength={160}
                          dir="ltr"
                          value={reference}
                          onChange={(e) => setReference(e.target.value)}
                        />
                      </label>
                    )}
                    {decision ? (
                      <div className="space-y-3 rounded-lg border border-amber-500/30 bg-amber-500/5 p-4">
                        <p className="text-[13px]">
                          {decision === "approve"
                            ? t(
                                "Confirm you have independently verified this transfer. This will deliver the subscription.",
                                "تأیید می‌کنید واریز را بررسی کرده‌اید؟ با این کار اشتراک تحویل داده می‌شود.",
                              )
                            : t(
                                "Reject this receipt with the reason above?",
                                "رسید با دلیل بالا رد شود؟",
                              )}
                        </p>
                        <div className="flex gap-2">
                          <Button
                            disabled={busy || !order}
                            onClick={() => void submit(decision)}
                          >
                            <Check className="h-4 w-4" />
                            {t("Confirm", "تأیید")}
                          </Button>
                          <Button
                            variant="outline"
                            disabled={busy}
                            onClick={() => setDecision(null)}
                          >
                            {t("Cancel", "انصراف")}
                          </Button>
                        </div>
                      </div>
                    ) : (
                      <div className="flex gap-2">
                        <Button
                          disabled={
                            busy ||
                            !order ||
                            (proof.method === "card" &&
                              reference.trim().length < 4)
                          }
                          onClick={() => setDecision("approve")}
                        >
                          {t("Approve transfer", "تأیید واریز")}
                        </Button>
                        <Button
                          variant="outline"
                          disabled={busy || !note.trim()}
                          onClick={() => setDecision("reject")}
                        >
                          {t("Reject receipt", "رد رسید")}
                        </Button>
                      </div>
                    )}
                  </>
                )}
              </>
            ) : (
              <>
                <div className="space-y-3">
                  {messages.map((m) => (
                    <div
                      key={m.id}
                      className={`rounded-xl border p-4 text-[13px] ${m.author === "staff" ? "border-emerald-500/20 bg-emerald-500/5" : "border-[var(--border)]"}`}
                    >
                      <p className="mb-2 text-[10px] text-[var(--muted)]">
                        {m.author === "staff"
                          ? t("Support", "پشتیبانی")
                          : t("Customer", "مشتری")}
                      </p>
                      <p className="whitespace-pre-wrap leading-7">{m.text}</p>
                    </div>
                  ))}
                </div>
                {editable && selected.state === "open" && (
                  <>
                    <label className="label">
                      {t("Reply", "پاسخ")}
                      <Textarea
                        maxLength={2000}
                        value={note}
                        onChange={(e) => setNote(e.target.value)}
                      />
                    </label>
                    <div className="flex gap-2">
                      <Button
                        disabled={busy || !note.trim()}
                        onClick={() => void submit("reply")}
                      >
                        {t("Send reply", "ارسال پاسخ")}
                      </Button>
                      <Button
                        variant="outline"
                        disabled={busy}
                        onClick={() => void submit("close")}
                      >
                        {t("Close ticket", "بستن تیکت")}
                      </Button>
                    </div>
                  </>
                )}
              </>
            )}
          </div>
        </Dialog>
      )}
    </div>
  );
}
