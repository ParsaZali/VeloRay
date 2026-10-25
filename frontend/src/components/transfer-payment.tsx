import { useState } from "react";
import { CheckCircle2, Upload, Copy } from "lucide-react";
import { api } from "@/lib/api";
import { commerceError, type Order } from "@/lib/commerce";
import type { PaymentProof } from "@/lib/sales";
import { t } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input, Textarea } from "@/components/ui/controls";
export function TransferPayment({
  order,
  token,
  onSubmitted,
}: {
  order: Order;
  token: string;
  onSubmitted: (proof: PaymentProof) => void;
}) {
  const methods = order.manual_payments || [];
  const [method, setMethod] = useState(methods[0]?.method || "card"),
    [image, setImage] = useState(""),
    [hash, setHash] = useState(""),
    [note, setNote] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const choice = methods.find((m) => m.method === method) || methods[0];
  async function file(file?: File) {
    setError("");
    if (!file) return;
    if (
      !["image/png", "image/jpeg", "image/webp"].includes(file.type) ||
      file.size > 1048576
    ) {
      setError(
        t(
          "Use a JPG, PNG or WebP photo up to 1 MiB.",
          "عکس JPG، PNG یا WebP تا یک مگابایت انتخاب کنید.",
        ),
      );
      return;
    }
    const reader = new FileReader();
    reader.onerror = () =>
      setError(t("Could not read this photo.", "خواندن عکس ناموفق بود."));
    reader.onload = () => setImage(String(reader.result));
    reader.readAsDataURL(file);
  }
  if (order.receipt?.state === "pending" || order.receipt?.state === "approved")
    return (
      <div
        role="status"
        className="space-y-2 rounded-lg border border-emerald-500/20 bg-emerald-500/5 p-4"
      >
        <CheckCircle2 className="h-5 w-5 text-emerald-500" />
        <p className="text-[13px]">
          {t(
            "Your receipt is awaiting review. We will deliver your subscription after approval.",
            "رسید شما در انتظار بررسی است. پس از تأیید، اشتراک تحویل داده می‌شود.",
          )}
        </p>
      </div>
    );
  if (!choice) return null;
  return (
    <form
      className="space-y-4 rounded-lg border border-[var(--border)] p-4"
      onSubmit={async (e) => {
        e.preventDefault();
        if (busy) return;
        setBusy(true);
        setError("");
        try {
          const proof = await api<PaymentProof>(
            `/shop/orders/${order.id}/receipt`,
            {
              method: "POST",
              headers: { Authorization: `Bearer ${token}` },
              body: JSON.stringify({
                method: choice.method,
                image,
                transaction_id: hash,
                note,
              }),
            },
          );
          onSubmitted(proof);
          setImage("");
          setHash("");
        } catch (e) {
          setError(commerceError(e));
        } finally {
          setBusy(false);
        }
      }}
    >
      <h3 className="text-[14px] font-semibold">
        {t("Transfer & send receipt", "واریز و ارسال رسید")}
      </h3>
      {order.receipt?.state === "rejected" && (
        <p className="rounded-lg bg-amber-500/10 p-3 text-[12px] text-amber-500">
          {t("Previous receipt rejected:", "رسید قبلی رد شد:")}{" "}
          {order.receipt.review_note}
        </p>
      )}
      {error && (
        <p role="alert" className="text-[12px] text-red-500">
          {error}
        </p>
      )}
      {methods.length > 1 && (
        <div className="flex gap-2">
          {methods.map((m) => (
            <Button
              key={m.method}
              type="button"
              variant={choice.method === m.method ? "default" : "outline"}
              onClick={() => setMethod(m.method)}
            >
              {m.method === "card"
                ? t("Bank transfer", "کارت‌به‌کارت")
                : "USDT"}
            </Button>
          ))}
        </div>
      )}
      <div className="space-y-2 rounded-lg bg-[var(--bg)] p-3">
        <p className="text-[12px] text-[var(--muted)]">
          {choice.method === "card"
            ? choice.card_holder
            : `USDT · ${choice.network}`}
        </p>
        <p dir="ltr" className="break-all font-mono text-[14px] select-all">
          {choice.card_number || choice.address}
        </p>
        <Button
          type="button"
          variant="ghost"
          onClick={() =>
            navigator.clipboard
              .writeText(choice.card_number || choice.address || "")
              .catch(() =>
                setError(
                  t(
                    "Select and copy the address.",
                    "آدرس را انتخاب و کپی کنید.",
                  ),
                ),
              )
          }
        >
          <Copy className="h-3.5 w-3.5" />
          {t("Copy", "کپی")}
        </Button>
        {choice.instructions && (
          <p className="whitespace-pre-wrap text-[12px] leading-6">
            {choice.instructions}
          </p>
        )}
      </div>
      {choice.method === "usdt" && (
        <>
          <p className="text-[12px] leading-6 text-amber-500">
            {t(
              "Use the exact network above. The administrator verifies the transaction before delivery.",
              "فقط از شبکه بالا استفاده کنید. مدیر تراکنش را پیش از تحویل بررسی می‌کند.",
            )}
          </p>
          <label className="label">
            {t("Transaction hash", "هش تراکنش")}
            <Input
              dir="ltr"
              required
              pattern="(0x)?[a-fA-F0-9]{64}"
              value={hash}
              onChange={(e) => setHash(e.target.value.trim())}
            />
          </label>
        </>
      )}
      <label className="label">
        <span className="flex items-center gap-2">
          <Upload className="h-4 w-4" />
          {t("Receipt photo · up to 1 MiB", "عکس رسید · تا یک مگابایت")}
        </span>
        <input
          className="w-full rounded-lg border border-[var(--border)] p-3 text-[12px]"
          type="file"
          accept="image/jpeg,image/png,image/webp"
          required={choice.method === "card" && !image}
          onChange={(e) => void file(e.target.files?.[0])}
        />
      </label>
      {image && (
        <img
          alt={t("Receipt preview", "پیش‌نمایش رسید")}
          src={image}
          className="max-h-52 max-w-full rounded-lg object-contain"
        />
      )}
      <label className="label">
        {t("Note · optional", "توضیح · اختیاری")}
        <Textarea
          maxLength={1200}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </label>
      <Button
        type="submit"
        disabled={busy || (choice.method === "card" && !image)}
      >
        {t(
          busy ? "Sending…" : "Submit receipt",
          busy ? "در حال ارسال…" : "ارسال رسید",
        )}
      </Button>
    </form>
  );
}
