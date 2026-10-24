import { useEffect, useState } from "react";
import {
  Package,
  Gift,
  RefreshCw,
  MessageCircle,
  BookOpen,
  Copy,
  ExternalLink,
} from "lucide-react";
import { api } from "@/lib/api";
import {
  amountLabel,
  commerceError,
  type Catalog,
  type Order,
} from "@/lib/commerce";
import type { BuyerService, Ticket, TicketMessage } from "@/lib/sales";
import { t } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input, Textarea } from "@/components/ui/controls";
import { Card, CardContent } from "@/components/ui/card";
type View = "services" | "orders" | "trial" | "support" | "tutorial";
export function BuyerHub({
  store,
  initData,
  catalog,
  onRenew,
  onOrder,
}: {
  store: string;
  initData: string;
  catalog: Catalog;
  onRenew: (service: BuyerService) => void;
  onOrder: (order: Order) => void;
}) {
  const [view, setView] = useState<View | null>(() =>
      new URLSearchParams(location.search).get("view") === "support"
        ? "support"
        : null,
    ),
    [services, setServices] = useState<BuyerService[]>([]),
    [orders, setOrders] = useState<Order[]>([]),
    [tickets, setTickets] = useState<Ticket[]>([]),
    [messages, setMessages] = useState<TicketMessage[]>([]),
    [ticket, setTicket] = useState<Ticket | null>(null),
    [text, setText] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [revision, setRevision] = useState(0),
    [terms, setTerms] = useState(false),
    [confirm, setConfirm] = useState<{
      service: BuyerService;
      action: string;
    } | null>(null);
  const base = `/shop/me`,
    scope = `?store=${encodeURIComponent(store)}`,
    headers = { "X-Telegram-Init-Data": initData };
  useEffect(() => {
    if (!view) return;
    const c = new AbortController();
    setError("");
    const path =
      view === "orders" ? "/orders" : view === "support" ? "/tickets" : "";
    void api<{ services: BuyerService[] } | Order[] | Ticket[]>(
      base + path + scope,
      { headers: { "X-Telegram-Init-Data": initData }, signal: c.signal },
    )
      .then((d) => {
        if (view === "orders") setOrders(d as Order[]);
        else if (view === "support") setTickets(d as Ticket[]);
        else if (!Array.isArray(d)) setServices(d.services);
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(commerceError(e));
      });
    return () => c.abort();
  }, [store, initData, view, revision]);
  useEffect(() => {
    setTicket(null);
    setMessages([]);
    setText("");
    setConfirm(null);
  }, [view]);
  useEffect(() => {
    if (!ticket) return;
    const c = new AbortController();
    void api<TicketMessage[]>(`${base}/tickets/${ticket.id}${scope}`, {
      headers: { "X-Telegram-Init-Data": initData },
      signal: c.signal,
    })
      .then(setMessages)
      .catch((e) => {
        if (!c.signal.aborted) setError(commerceError(e));
      });
    return () => c.abort();
  }, [ticket, revision, store, initData]);
  async function mutate(path: string, body: unknown) {
    if (busy) return false;
    setBusy(true);
    setError("");
    try {
      await api(base + path + scope, {
        method: "POST",
        headers,
        body: JSON.stringify(body),
      });
      setRevision((v) => v + 1);
      setConfirm(null);
      setText("");
      return true;
    } catch (e) {
      setError(commerceError(e));
      return false;
    } finally {
      setBusy(false);
    }
  }
  const baseChoices: [View, string, typeof Package][] = [
    ["services", t("My services", "سرویس‌های من"), Package],
    ["orders", t("My orders", "سفارش‌های من"), RefreshCw],
    ...(catalog.bot?.trial_enabled
      ? [
          ["trial", t("Free trial", "تست رایگان"), Gift] as [
            View,
            string,
            typeof Package,
          ],
        ]
      : []),
    ["tutorial", t("Tutorials", "آموزش اتصال"), BookOpen],
    ["support", t("Support", "پشتیبانی"), MessageCircle],
  ];
  const configured = catalog.bot?.buttons;
  const actionFor = (view: View) => (view === "orders" ? "account" : view);
  const choices = baseChoices
    .filter(
      ([key]) =>
        !configured ||
        configured.find((b) => b.action === actionFor(key))?.visible !== false,
    )
    .map(
      ([key, label, Icon]) =>
        [
          key,
          configured?.find((b) => b.action === actionFor(key))?.label || label,
          Icon,
        ] as [View, string, typeof Package],
    )
    .sort(
      (a, b) =>
        (configured?.find((v) => v.action === actionFor(a[0]))?.row || 0) -
        (configured?.find((v) => v.action === actionFor(b[0]))?.row || 0),
    );
  const bytes = (n: number) =>
    `${(n / 1073741824).toLocaleString(undefined, { maximumFractionDigits: 3 })} GiB`;
  return (
    <section
      aria-label={t("Customer area", "حساب مشتری")}
      className="space-y-4"
    >
      <nav className="flex flex-wrap gap-2">
        {choices.map(([key, label, Icon]) => (
          <Button
            key={key}
            variant={view === key ? "default" : "outline"}
            onClick={() => setView(view === key ? null : key)}
          >
            <Icon className="h-4 w-4" />
            {label}
          </Button>
        ))}
      </nav>
      {catalog.bot?.custom_buttons?.length ? (
        <div className="flex flex-wrap gap-2">
          {catalog.bot.custom_buttons.map((b, i) =>
            b.url ? (
              <a
                key={i}
                className="rounded-lg border border-[var(--border)] px-4 py-2 text-[12px] text-emerald-500"
                href={b.url}
                target="_blank"
                rel="noreferrer"
              >
                {b.label}
              </a>
            ) : (
              <details
                key={i}
                className="min-w-0 rounded-lg border border-[var(--border)] p-3 text-[12px]"
              >
                <summary className="cursor-pointer">{b.label}</summary>
                {b.image && (
                  <img
                    alt=""
                    src={b.image}
                    className="mt-3 max-h-52 rounded-lg object-contain"
                  />
                )}
                <p className="mt-3 whitespace-pre-wrap leading-7">{b.text}</p>
              </details>
            ),
          )}
        </div>
      ) : null}
      {view && (
        <Card>
          <CardContent className="space-y-4">
            <div className="flex justify-between">
              <h2 className="font-semibold">
                {choices.find((v) => v[0] === view)?.[1]}
              </h2>
              <Button
                aria-label={t("Refresh", "بروزرسانی")}
                variant="ghost"
                onClick={() => setRevision((v) => v + 1)}
              >
                <RefreshCw className="h-4 w-4" />
              </Button>
            </div>
            {error && (
              <p role="alert" className="text-[12px] text-red-500">
                {error}
              </p>
            )}
            {view === "services" && (
              <>
                {!services.length && (
                  <p className="text-[13px] text-[var(--muted)]">
                    {catalog.bot?.empty_text ||
                      t(
                        "You have no subscriptions yet.",
                        "هنوز اشتراکی ندارید.",
                      )}
                  </p>
                )}
                {services.map((s) => (
                  <div
                    key={s.id}
                    className="space-y-3 rounded-xl border border-[var(--border)] p-4"
                  >
                    <div className="flex items-center justify-between gap-3">
                      <h3 className="text-[14px] font-semibold">{s.name}</h3>
                      <span
                        className={`text-[11px] ${s.enabled ? "text-emerald-500" : "text-amber-500"}`}
                      >
                        {s.enabled
                          ? t("Enabled", "فعال")
                          : t("Disabled", "غیرفعال")}
                      </span>
                    </div>
                    <dl className="grid grid-cols-3 gap-3 text-[12px]">
                      <div>
                        <dt className="text-[var(--muted)]">
                          {t("Used", "مصرف")}
                        </dt>
                        <dd className="mt-1">{bytes(s.used_traffic_bytes)}</dd>
                      </div>
                      <div>
                        <dt className="text-[var(--muted)]">
                          {t("Remaining", "باقی‌مانده")}
                        </dt>
                        <dd className="mt-1">
                          {s.unlimited
                            ? t("Unlimited", "نامحدود")
                            : bytes(s.remaining_bytes || 0)}
                        </dd>
                      </div>
                      <div>
                        <dt className="text-[var(--muted)]">
                          {t("Expiry", "انقضا")}
                        </dt>
                        <dd className="mt-1">
                          {s.expires_at
                            ? new Date(s.expires_at).toLocaleDateString()
                            : "—"}
                        </dd>
                      </div>
                    </dl>
                    <Input
                      aria-label={t("Subscription link", "لینک اشتراک")}
                      readOnly
                      dir="ltr"
                      value={s.subscription_url}
                      onFocus={(e) => e.target.select()}
                    />
                    <div className="flex flex-wrap gap-2">
                      <Button
                        variant="outline"
                        onClick={() =>
                          navigator.clipboard
                            .writeText(s.subscription_url)
                            .catch(() =>
                              setError(
                                t(
                                  "Select and copy the link.",
                                  "لینک را انتخاب و کپی کنید.",
                                ),
                              ),
                            )
                        }
                      >
                        <Copy className="h-4 w-4" />
                        {t("Copy link", "کپی لینک")}
                      </Button>
                      <Button onClick={() => onRenew(s)}>
                        {t("Renew / add traffic", "تمدید / افزایش حجم")}
                      </Button>
                      {catalog.bot?.allow_toggle && (
                        <Button
                          variant="outline"
                          onClick={() =>
                            setConfirm({
                              service: s,
                              action: s.enabled ? "disable" : "enable",
                            })
                          }
                        >
                          {s.enabled
                            ? t("Disable", "خاموش کردن")
                            : t("Enable", "روشن کردن")}
                        </Button>
                      )}
                      {catalog.bot?.allow_rotate && (
                        <Button
                          variant="outline"
                          onClick={() =>
                            setConfirm({ service: s, action: "rotate" })
                          }
                        >
                          {t("Change link", "تغییر لینک")}
                        </Button>
                      )}
                    </div>
                    {confirm?.service.id === s.id && (
                      <div className="space-y-3 rounded-lg bg-amber-500/10 p-3">
                        <p className="text-[12px]">
                          {confirm.action === "rotate"
                            ? t(
                                "Change this subscription link? Old links will stop working.",
                                "لینک تغییر کند؟ لینک‌های قبلی از کار می‌افتند.",
                              )
                            : t(
                                "Confirm this service change?",
                                "تغییر وضعیت سرویس تأیید شود؟",
                              )}
                        </p>
                        <div className="flex gap-2">
                          <Button
                            disabled={busy}
                            onClick={() =>
                              void mutate(
                                `/services/${s.id}/${confirm.action}`,
                                { confirm: true },
                              )
                            }
                          >
                            {t("Confirm", "تأیید")}
                          </Button>
                          <Button
                            variant="outline"
                            onClick={() => setConfirm(null)}
                          >
                            {t("Cancel", "انصراف")}
                          </Button>
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </>
            )}
            {view === "orders" && (
              <>
                {!orders.length && (
                  <p className="text-[13px] text-[var(--muted)]">
                    {t("No orders yet.", "هنوز سفارشی ندارید.")}
                  </p>
                )}
                {orders.map((o) => (
                  <button
                    className="flex w-full justify-between gap-3 rounded-lg border border-[var(--border)] p-4 text-start text-[12px]"
                    key={o.id}
                    onClick={() => onOrder(o)}
                  >
                    <span>
                      {o.snapshot?.name || o.id.slice(0, 8)} · {o.state}
                    </span>
                    <span>{amountLabel(o.amount, o.currency)}</span>
                  </button>
                ))}
              </>
            )}
            {view === "trial" && (
              <>
                <p className="text-[13px] leading-7">
                  {catalog.bot?.trial_text ||
                    t(
                      "Get a trial subscription. Each Telegram account can claim it once.",
                      "اشتراک آزمایشی دریافت کنید. هر حساب تلگرام یک‌بار می‌تواند تست بگیرد.",
                    )}
                </p>
                {catalog.bot?.trial_channel_url && (
                  <a
                    className="inline-block text-[12px] text-emerald-500"
                    href={catalog.bot.trial_channel_url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {t("Join trial channel", "عضویت در کانال دریافت تست")}
                  </a>
                )}
                <label className="flex gap-2 text-[12px]">
                  <input
                    type="checkbox"
                    checked={terms}
                    onChange={(e) => setTerms(e.target.checked)}
                  />
                  <span>
                    {t("I accept", "می‌پذیرم:")}{" "}
                    <a
                      className="text-emerald-500"
                      href={catalog.terms_url}
                      target="_blank"
                      rel="noreferrer"
                    >
                      {t("purchase terms", "شرایط خرید")}
                    </a>
                  </span>
                </label>
                <Button
                  disabled={busy || !terms}
                  onClick={() =>
                    void mutate("/trial", { accept_terms: terms }).then(
                      (ok) => {
                        if (ok) setView("services");
                      },
                    )
                  }
                >
                  <Gift className="h-4 w-4" />
                  {t("Get free trial", "دریافت تست رایگان")}
                </Button>
              </>
            )}
            {view === "tutorial" && (
              <>
                {catalog.bot?.tutorial_image && (
                  <img
                    alt=""
                    src={catalog.bot.tutorial_image}
                    className="max-h-52 rounded-lg object-contain"
                  />
                )}
                <p className="whitespace-pre-wrap text-[13px] leading-7">
                  {catalog.bot?.tutorial_text}
                </p>
                {catalog.bot?.tutorials?.map((d, i) => (
                  <details
                    key={i}
                    className="rounded-lg border border-[var(--border)] p-4"
                  >
                    <summary className="cursor-pointer text-[13px] font-medium">
                      {d.title}
                    </summary>
                    <div className="mt-4 space-y-3">
                      {d.image && (
                        <img
                          alt=""
                          src={d.image}
                          className="max-h-64 rounded-lg object-contain"
                        />
                      )}
                      <p className="whitespace-pre-wrap text-[13px] leading-7">
                        {d.text}
                      </p>
                      {d.url && (
                        <a
                          href={d.url}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex items-center gap-2 text-[12px] text-emerald-500"
                        >
                          <ExternalLink className="h-4 w-4" />
                          {t("Open link", "باز کردن لینک")}
                        </a>
                      )}
                    </div>
                  </details>
                ))}
              </>
            )}
            {view === "support" && (
              <>
                <p className="whitespace-pre-wrap text-[13px]">
                  {catalog.bot?.support_text}
                </p>
                {catalog.support_url && (
                  <a
                    className="inline-block text-[12px] text-emerald-500"
                    href={catalog.support_url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {t("Contact support", "تماس با پشتیبانی")}
                  </a>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    onClick={() => {
                      setTicket(null);
                      setMessages([]);
                    }}
                  >
                    {t("New ticket", "تیکت جدید")}
                  </Button>
                  {tickets.map((d) => (
                    <Button
                      key={d.id}
                      variant={d.id === ticket?.id ? "default" : "outline"}
                      onClick={() => setTicket(d)}
                    >
                      #{d.id} · {d.subject}
                    </Button>
                  ))}
                </div>
                {messages.map((m) => (
                  <div
                    key={m.id}
                    className={`rounded-lg border p-3 text-[13px] whitespace-pre-wrap leading-7 ${m.author === "staff" ? "border-emerald-500/20 bg-emerald-500/5" : "border-[var(--border)]"}`}
                  >
                    {m.text}
                  </div>
                ))}
                {(!ticket || ticket.state === "open") && (
                  <form
                    className="space-y-3"
                    onSubmit={(e) => {
                      e.preventDefault();
                      void mutate(`/tickets${ticket ? `/${ticket.id}` : ""}`, {
                        text,
                      });
                    }}
                  >
                    <label className="label">
                      {t("Your message", "پیام شما")}
                      <Textarea
                        maxLength={2000}
                        required
                        value={text}
                        onChange={(e) => setText(e.target.value)}
                      />
                    </label>
                    <Button disabled={busy || !text.trim()}>
                      {t("Send", "ارسال")}
                    </Button>
                  </form>
                )}
              </>
            )}
          </CardContent>
        </Card>
      )}
    </section>
  );
}
