import { lazy, Suspense, useEffect, useState, type FormEvent } from "react";
import {
  Check,
  CheckCircle2,
  Copy,
  Globe,
  Moon,
  Package,
  RefreshCw,
  ShieldCheck,
  Send,
  Sun,
} from "lucide-react";
import { api } from "@/lib/api";
import {
  amountLabel,
  commerceError,
  rememberOrder,
  tokenFromURL,
  type Catalog,
  type Offer,
  type Order,
} from "@/lib/commerce";
import { t, useLocale } from "@/lib/i18n";
import { useTheme } from "@/components/theme/theme-provider";
import { VeloRayMark } from "@/components/brand/veloray-mark";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/controls";
import { Card, CardContent } from "@/components/ui/card";
import { orderStatus } from "@/lib/commerce";
import {
  loadMiniApp,
  miniAppData,
  openStarsInvoice,
} from "@/lib/telegram-miniapp";

const BuyerHub = lazy(() =>
  import("@/components/buyer-hub").then((m) => ({ default: m.BuyerHub })),
);
const TransferPayment = lazy(() =>
  import("@/components/transfer-payment").then((m) => ({
    default: m.TransferPayment,
  })),
);
export function StorefrontPage() {
  const { locale, setLocale } = useLocale(),
    { theme, setTheme } = useTheme();
  const params = new URLSearchParams(window.location.search),
    store = params.get("store") || "",
    initialOrder = params.get("order") || "",
    requestedService = params.get("service") || "";
  const [telegramData] = useState(miniAppData),
    [termsAccepted, setTermsAccepted] = useState(false),
    [privateToken, setPrivateToken] = useState<string | null>(() => {
      const token = new URLSearchParams(location.hash.slice(1)).get("token");
      return initialOrder && token && /^[A-Za-z0-9_-]{32,128}$/.test(token)
        ? token
        : null;
    });
  const [catalog, setCatalog] = useState<Catalog | null>(null),
    [selected, setSelected] = useState<Offer | null>(null),
    [kind, setKind] = useState("purchase"),
    [category, setCategory] = useState(""),
    [name, setName] = useState(""),
    [subscription, setSubscription] = useState("");
  const [order, setOrder] = useState<Order | null>(null),
    [orderID, setOrderID] = useState(initialOrder),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [copied, setCopied] = useState(false);
  const orderToken = orderID
    ? privateToken || sessionStorage.getItem(`veloray-order:${orderID}`)
    : null;
  useEffect(() => {
    const fragment = new URLSearchParams(location.hash.slice(1));
    const token = fragment.get("token");
    if (initialOrder && token && /^[A-Za-z0-9_-]{32,128}$/.test(token)) {
      sessionStorage.setItem(`veloray-order:${initialOrder}`, token);
      history.replaceState({}, "", location.pathname + location.search);
    }
    if (telegramData) void loadMiniApp().catch(() => {});
  }, [initialOrder, telegramData]);
  useEffect(() => {
    if (initialOrder) {
      setLoading(false);
      return;
    }
    const c = new AbortController();
    void api<Catalog>(`/shop/catalog?store=${encodeURIComponent(store)}`, {
      signal: c.signal,
      ...(telegramData
        ? { headers: { "X-Telegram-Init-Data": telegramData } }
        : {}),
    })
      .then((d) => {
        setCatalog(d);
        setSelected(d.offers[0] || null);
        document.title = d.name;
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(commerceError(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [store, initialOrder, telegramData]);
  useEffect(() => {
    if (!requestedService || !telegramData || !catalog) return;
    const c = new AbortController();
    void api<{ services: import("@/lib/sales").BuyerService[] }>(
      `/shop/me?store=${encodeURIComponent(store)}`,
      { headers: { "X-Telegram-Init-Data": telegramData }, signal: c.signal },
    )
      .then((d) => {
        const own = d.services.find((s) => String(s.id) === requestedService);
        if (!own)
          throw new Error(t("Subscription not found", "اشتراک پیدا نشد"));
        setKind("renewal");
        setSubscription(own.subscription_url);
        const match = catalog.offers.find((o) =>
          own.service_ids?.includes(o.service_id),
        );
        if (match) setSelected(match);
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(commerceError(e));
      });
    return () => c.abort();
  }, [requestedService, telegramData, store, catalog]);
  useEffect(() => {
    if (!orderID) return;
    if (!orderToken) {
      setError(
        t(
          "Open this order in the browser where you created it.",
          "سفارش را در همان مرورگری باز کنید که آن را ساختید.",
        ),
      );
      return;
    }
    const c = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function refresh() {
      try {
        const result = await api<Order>(`/shop/orders/${orderID}`, {
          headers: { Authorization: `Bearer ${orderToken}` },
          signal: c.signal,
        });
        if (c.signal.aborted) return;
        setOrder(result);
        if (
          (result.state === "paid" ||
            (result.state === "pending" &&
              (result.currency === "XTR" ||
                result.receipt?.state === "pending"))) &&
          !result.test_mode
        )
          timer = setTimeout(() => void refresh(), 5000);
      } catch (e) {
        if (!c.signal.aborted) setError(commerceError(e));
      }
    }
    void refresh();
    return () => {
      c.abort();
      clearTimeout(timer);
    };
  }, [orderID, orderToken]);
  async function pay(id: string, token: string) {
    const response = await api<{ url?: string; gateway?: string }>(
      `/shop/orders/${id}/pay`,
      {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
        body: "{}",
      },
    );
    if (response.gateway === "manual") return;
    if (!response.url)
      throw new Error(t("Payment link unavailable", "لینک پرداخت موجود نیست"));
    if (response.gateway === "telegram-stars" && telegramData) {
      await openStarsInvoice(response.url, () => {
        void api<Order>(`/shop/orders/${id}`, {
          headers: { Authorization: `Bearer ${token}` },
        })
          .then(setOrder)
          .catch((e) => setError(commerceError(e)));
      });
    } else window.location.assign(response.url);
  }
  async function checkout(e: FormEvent) {
    e.preventDefault();
    if (!selected || busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await api<Order>("/shop/orders", {
        method: "POST",
        body: JSON.stringify({
          offer_id: selected.id,
          name,
          kind,
          customer_token: tokenFromURL(subscription),
          ...(telegramData
            ? { telegram_init_data: telegramData, accept_terms: termsAccepted }
            : {}),
        }),
      });
      rememberOrder(result);
      setPrivateToken(result.token || null);
      setOrderID(result.id);
      history.replaceState({}, "", `/shop?store=${store}&order=${result.id}`);
      await pay(result.id, result.token!);
    } catch (e) {
      setError(commerceError(e));
    } finally {
      setBusy(false);
    }
  }
  async function verify() {
    if (!orderToken || busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await api<Order>(`/shop/orders/${orderID}`, {
        headers: { Authorization: `Bearer ${orderToken}` },
      });
      setOrder(result);
    } catch (e) {
      setError(commerceError(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="min-h-screen bg-[var(--bg)] text-[var(--fg)]">
      <header className="border-b border-[var(--border)] bg-[var(--card)]">
        <div className="mx-auto flex max-w-5xl items-center justify-between gap-3 px-5 py-4">
          <div className="flex min-w-0 items-center gap-3">
            {catalog?.logo_url ? (
              <img
                alt=""
                src={catalog.logo_url}
                className="h-9 w-9 rounded-lg object-contain"
              />
            ) : (
              <VeloRayMark className="h-9 w-9" />
            )}
            <span className="truncate font-semibold">
              {catalog?.name || "VeloRay"}
            </span>
          </div>
          <div className="flex gap-2">
            <Button
              variant="ghost"
              onClick={() => setLocale(locale === "fa" ? "en" : "fa")}
              aria-label={t("Change language", "تغییر زبان")}
            >
              <Globe className="h-4 w-4" />
              <span className="text-[12px]">
                {locale === "fa" ? "EN" : "فارسی"}
              </span>
            </Button>
            <Button
              variant="ghost"
              onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
              aria-label={t("Change theme", "تغییر تم")}
            >
              {theme === "dark" ? (
                <Sun className="h-4 w-4" />
              ) : (
                <Moon className="h-4 w-4" />
              )}
            </Button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-5xl space-y-6 px-5 py-8 sm:py-12">
        {error && (
          <p
            role="alert"
            className="rounded-lg border border-red-500/20 bg-red-500/5 p-4 text-[13px] text-red-500"
          >
            {error}
          </p>
        )}
        {loading ? (
          <div role="status" className="py-20 text-center text-[var(--muted)]">
            {t("Loading…")}
          </div>
        ) : orderID ? (
          <div className="mx-auto max-w-xl">
            <Card>
              <CardContent className="space-y-5 sm:p-7">
                <div className="flex items-center gap-3">
                  {order?.state === "fulfilled" ? (
                    <CheckCircle2 className="h-7 w-7 text-emerald-500" />
                  ) : (
                    <Package className="h-7 w-7 text-emerald-500" />
                  )}
                  <div>
                    <h1 className="text-lg font-semibold">
                      {t("Your order", "سفارش شما")}
                    </h1>
                    <p className="mt-1 font-mono text-[11px] text-[var(--muted)]">
                      {orderID}
                    </p>
                  </div>
                </div>
                {order && (
                  <>
                    <dl className="grid grid-cols-2 gap-4 border-y border-[var(--border)] py-5 text-[13px]">
                      <div>
                        <dt className="text-[var(--muted)]">
                          {t("Amount", "مبلغ")}
                        </dt>
                        <dd className="mt-1 font-semibold">
                          {amountLabel(order.amount, order.currency)}
                        </dd>
                      </div>
                      <div>
                        <dt className="text-[var(--muted)]">
                          {t("Status", "وضعیت")}
                        </dt>
                        <dd className="mt-1 text-emerald-500">
                          {order.test_mode
                            ? t("Test payment", "پرداخت آزمایشی")
                            : orderStatus(order.state)}
                        </dd>
                      </div>
                    </dl>
                    {order.snapshot?.name && (
                      <p className="text-[13px]">{order.snapshot.name}</p>
                    )}
                    {order.subscription_url && (
                      <>
                        <label className="label">
                          {t("Subscription link", "لینک اشتراک")}
                          <Input
                            className="input"
                            readOnly
                            value={order.subscription_url}
                            dir="ltr"
                            onFocus={(e) => e.target.select()}
                          />
                        </label>
                        <Button
                          onClick={async () => {
                            try {
                              await navigator.clipboard.writeText(
                                order.subscription_url!,
                              );
                              setCopied(true);
                            } catch {
                              setError(
                                t(
                                  "Select and copy the link above.",
                                  "لینک بالا را انتخاب و کپی کنید.",
                                ),
                              );
                            }
                          }}
                        >
                          {copied ? (
                            <Check className="h-4 w-4" />
                          ) : (
                            <Copy className="h-4 w-4" />
                          )}
                          {t(
                            copied ? "Copied" : "Copy link",
                            copied ? "کپی شد" : "کپی لینک",
                          )}
                        </Button>
                      </>
                    )}
                    {order.test_mode && (
                      <p className="text-[12px] text-amber-500">
                        {t(
                          "This test did not create a subscription or wallet credit.",
                          "این آزمایش اشتراک یا اعتبار کیف پول ایجاد نکرده است.",
                        )}
                      </p>
                    )}
                    {order.state === "paid" && !order.test_mode && (
                      <p
                        role="status"
                        className="text-[13px] text-[var(--muted)]"
                      >
                        {t(
                          "Payment received. Your subscription is being prepared.",
                          "پرداخت تأیید شد. اشتراک در حال آماده‌سازی است.",
                        )}
                      </p>
                    )}
                    {order.state === "pending" &&
                      orderToken &&
                      order.currency !== "XTR" && (
                        <Suspense
                          fallback={<p role="status">{t("Loading…")}</p>}
                        >
                          <TransferPayment
                            key={order.id}
                            order={order}
                            token={orderToken}
                            onSubmitted={(proof) =>
                              setOrder((current) =>
                                current
                                  ? { ...current, receipt: proof }
                                  : current,
                              )
                            }
                          />
                        </Suspense>
                      )}
                    {order.state === "pending" &&
                      orderToken &&
                      order.currency === "XTR" && (
                        <div className="flex flex-wrap gap-2">
                          <Button
                            disabled={busy}
                            onClick={async () => {
                              setBusy(true);
                              setError("");
                              try {
                                await pay(orderID, orderToken);
                              } catch (e) {
                                setError(commerceError(e));
                              } finally {
                                setBusy(false);
                              }
                            }}
                          >
                            {t("Continue payment", "ادامهٔ پرداخت")}
                          </Button>
                          <Button
                            disabled={busy}
                            variant="outline"
                            onClick={() => void verify()}
                          >
                            <RefreshCw className="h-4 w-4" />
                            {t("Check payment", "بررسی پرداخت")}
                          </Button>
                        </div>
                      )}
                  </>
                )}
                <a
                  href={`/shop?store=${store}`}
                  className="inline-block text-[12px] text-[var(--muted)] hover:text-emerald-500"
                >
                  {t("Back to store", "بازگشت به فروشگاه")}
                </a>
              </CardContent>
            </Card>
          </div>
        ) : (
          catalog && (
            <>
              {telegramData && (
                <Suspense fallback={<p role="status">{t("Loading…")}</p>}>
                  <BuyerHub
                    store={store}
                    initData={telegramData}
                    catalog={catalog}
                    onRenew={(service) => {
                      setKind("renewal");
                      setSubscription(service.subscription_url);
                      const matching = catalog.offers.find((o) =>
                        service.service_ids?.includes(o.service_id),
                      );
                      if (matching) setSelected(matching);
                      document
                        .getElementById("store-checkout")
                        ?.scrollIntoView({
                          behavior: "smooth",
                          block: "start",
                        });
                    }}
                    onOrder={(o) => {
                      rememberOrder(o);
                      setPrivateToken(o.token || null);
                      setOrderID(o.id);
                      history.replaceState(
                        {},
                        "",
                        `/shop?store=${store}&order=${o.id}`,
                      );
                    }}
                  />
                </Suspense>
              )}
              <div className="max-w-2xl">
                <h1 className="text-2xl font-semibold tracking-tight">
                  {catalog.bot?.catalog_text?.replaceAll(
                    "{store}",
                    catalog.name,
                  ) ||
                    t("Choose your subscription", "اشتراک خود را انتخاب کنید")}
                </h1>
                {catalog.bot?.welcome_image && (
                  <img
                    alt=""
                    src={catalog.bot.welcome_image}
                    className="mb-4 max-h-52 rounded-xl object-contain"
                  />
                )}
                {catalog.bot?.welcome && (
                  <p className="mt-3 whitespace-pre-wrap text-[14px] leading-7">
                    {catalog.bot.welcome.replaceAll("{store}", catalog.name)}
                  </p>
                )}
                {catalog.description && (
                  <p className="mt-3 text-[14px] leading-7 text-[var(--muted)]">
                    {catalog.description}
                  </p>
                )}
              </div>
              {[
                ...new Set(
                  catalog.offers.map((o) => o.category).filter(Boolean),
                ),
              ].length > 0 && (
                <nav
                  aria-label={t("Categories", "دسته‌بندی‌ها")}
                  className="flex flex-wrap gap-2"
                >
                  <Button
                    variant={!category ? "default" : "outline"}
                    onClick={() => setCategory("")}
                  >
                    {t("All", "همه")}
                  </Button>
                  {[
                    ...new Set(
                      catalog.offers.map((o) => o.category).filter(Boolean),
                    ),
                  ].map((c) => (
                    <Button
                      key={c}
                      variant={category === c ? "default" : "outline"}
                      onClick={() => setCategory(c || "")}
                    >
                      {c}
                    </Button>
                  ))}
                </nav>
              )}
              <div className="grid items-start gap-6 lg:grid-cols-[1fr_340px]">
                <section
                  aria-label={t("Subscription plans", "بسته‌های اشتراک")}
                  className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1"
                >
                  {catalog.offers
                    .filter((o) => !category || o.category === category)
                    .map((o) => (
                      <button
                        key={o.id}
                        type="button"
                        aria-pressed={selected?.id === o.id}
                        onClick={() => setSelected(o)}
                        className={`rounded-xl border bg-[var(--card)] p-5 text-start transition-colors ${selected?.id === o.id ? "border-emerald-500 ring-1 ring-emerald-500/20" : "border-[var(--border)] hover:border-emerald-500/50"}`}
                      >
                        <div className="flex items-center justify-between gap-3">
                          <div>
                            {o.image_url && (
                              <img
                                alt=""
                                loading="lazy"
                                src={o.image_url}
                                className="mb-3 h-28 w-full rounded-lg object-cover"
                              />
                            )}
                            <h2 className="font-semibold">{o.name}</h2>
                          </div>
                          <span
                            className={`grid h-5 w-5 shrink-0 place-items-center rounded-full border ${selected?.id === o.id ? "border-emerald-500 bg-emerald-500 text-white" : "border-[var(--border)]"}`}
                          >
                            {selected?.id === o.id && (
                              <Check className="h-3 w-3" />
                            )}
                          </span>
                        </div>
                        <div className="mt-4 flex flex-wrap items-baseline justify-between gap-3">
                          <p className="text-xl font-semibold">
                            {amountLabel(o.amount, o.currency)}
                          </p>
                          <p className="text-[12px] text-[var(--muted)]">
                            {o.duration_days} {t("days", "روز")} ·{" "}
                            {o.traffic_bytes
                              ? `${(o.traffic_bytes / 1073741824).toLocaleString()} GiB`
                              : t("Unlimited", "نامحدود")}
                          </p>
                        </div>
                        {o.description && (
                          <p className="mt-3 text-[12px] leading-6 text-[var(--muted)]">
                            {o.description}
                          </p>
                        )}
                      </button>
                    ))}
                  {!catalog.offers.length && (
                    <p className="py-10 text-[var(--muted)]">
                      {t(
                        "No plans available yet.",
                        "هنوز بسته‌ای برای فروش منتشر نشده.",
                      )}
                    </p>
                  )}
                </section>
                <Card>
                  <CardContent className="space-y-5">
                    <h2 className="font-semibold">
                      {t("Checkout", "خرید اشتراک")}
                    </h2>
                    <form
                      id="store-checkout"
                      onSubmit={checkout}
                      className="space-y-4"
                    >
                      <label className="label">
                        {t("Action", "عملیات")}
                        <Select
                          className="input"
                          value={kind}
                          onChange={(e) => setKind(e.target.value)}
                        >
                          <option value="purchase">
                            {t("New subscription", "اشتراک جدید")}
                          </option>
                          <option value="renewal">
                            {t("Renew subscription", "تمدید اشتراک")}
                          </option>
                          <option value="traffic">
                            {t("Add traffic", "افزایش حجم")}
                          </option>
                        </Select>
                      </label>
                      {kind === "purchase" ? (
                        <label className="label">
                          {t("Your name", "نام شما")}
                          <Input
                            className="input"
                            required
                            autoComplete="name"
                            maxLength={80}
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                          />
                        </label>
                      ) : (
                        <label className="label">
                          {t("Subscription URL or token", "لینک یا شناسهٔ ساب")}
                          <Input
                            className="input"
                            required
                            type="password"
                            autoComplete="off"
                            value={subscription}
                            onChange={(e) => setSubscription(e.target.value)}
                          />
                        </label>
                      )}
                      <div className="flex justify-between border-t border-[var(--border)] pt-4 text-[13px]">
                        <span className="text-[var(--muted)]">
                          {t("Total", "مبلغ نهایی")}
                        </span>
                        <strong>
                          {selected
                            ? amountLabel(selected.amount, selected.currency)
                            : "—"}
                        </strong>
                      </div>
                      {catalog.telegram_checkout && catalog.terms_url && (
                        <label className="flex items-start gap-2 text-[12px] leading-6">
                          <input
                            className="mt-1"
                            type="checkbox"
                            required
                            checked={termsAccepted}
                            onChange={(e) => setTermsAccepted(e.target.checked)}
                          />
                          <span>
                            {t("I accept the store's", "می‌پذیرم:")}{" "}
                            <a
                              className="text-emerald-500"
                              href={catalog.terms_url}
                              target="_blank"
                              rel="noreferrer"
                            >
                              {t("purchase terms", "شرایط خرید فروشگاه")}
                            </a>
                          </span>
                        </label>
                      )}
                      <Button
                        className="w-full"
                        type="submit"
                        disabled={
                          busy ||
                          !selected ||
                          !catalog.payments_enabled ||
                          (!!catalog.telegram_checkout && !termsAccepted)
                        }
                      >
                        {t(
                          busy ? "Please wait…" : "Continue to payment",
                          busy ? "لطفاً صبر کنید…" : "ادامه و پرداخت",
                        )}
                      </Button>
                      {!catalog.payments_enabled && (
                        <p className="text-[11px] text-amber-500">
                          {t(
                            "Online payments are currently unavailable.",
                            "پرداخت آنلاین در حال حاضر فعال نیست.",
                          )}
                        </p>
                      )}
                      {!catalog.telegram_checkout && catalog.telegram_url && (
                        <a
                          className="flex items-center gap-2 text-[12px] text-emerald-500"
                          href={catalog.telegram_url}
                          target="_blank"
                          rel="noreferrer"
                        >
                          <Send className="h-4 w-4" />
                          {t(
                            "Buy with Telegram Stars",
                            "خرید با استارز تلگرام",
                          )}
                        </a>
                      )}
                      <p className="flex items-center gap-2 text-[11px] text-[var(--muted)]">
                        <ShieldCheck className="h-4 w-4 text-emerald-500" />
                        {t(
                          "Delivered after payment verification",
                          "تحویل پس از تأیید پرداخت",
                        )}
                      </p>
                    </form>
                    {catalog.support_url && (
                      <a
                        className="block border-t border-[var(--border)] pt-4 text-[12px] text-emerald-500"
                        href={catalog.support_url}
                        target="_blank"
                        rel="noreferrer"
                      >
                        {t("Contact support", "ارتباط با پشتیبانی")}
                      </a>
                    )}
                  </CardContent>
                </Card>
              </div>
            </>
          )
        )}
      </main>
      <footer className="mx-auto max-w-5xl px-5 pb-6 text-[11px] text-[var(--muted)]">
        VeloRay
      </footer>
    </div>
  );
}
