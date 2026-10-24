import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import {
  ArrowDownLeft,
  ArrowUpRight,
  ExternalLink,
  Package,
  Plus,
  RefreshCw,
  ShoppingBag,
  Wallet as WalletIcon,
} from "lucide-react";
import { api, type AdminUser } from "@/lib/api";
import { can, useSession } from "@/lib/session";
import { t, useLocale } from "@/lib/i18n";
import {
  orderStatus,
  amountLabel,
  commerceError,
  rememberOrder,
  tokenFromURL,
  type LedgerEntry,
  type Offer,
  type Order,
  type Reports,
  type StoreBrand,
  type Wallet,
} from "@/lib/commerce";
import { Button } from "@/components/ui/button";
import { Input, Select, Textarea } from "@/components/ui/controls";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog } from "@/components/common/dialog";
import { CommerceBreakdown } from "@/components/commerce-breakdown";
import { StoreDomain } from "@/components/store-domain";
import { StoreTelegram, SalesPayments } from "@/components/store-telegram";

const SalesReview = lazy(() =>
  import("@/components/sales-review").then((m) => ({ default: m.SalesReview })),
);
const ResellerTeam = lazy(() =>
  import("@/components/reseller-team").then((module) => ({
    default: module.ResellerTeam,
  })),
);
type Tab =
  | "overview"
  | "offers"
  | "orders"
  | "wallet"
  | "store"
  | "gateway"
  | "receipts"
  | "tickets"
  | "team";
const emptyStore: StoreBrand = {
  enabled: false,
  name: "VeloRay",
  description: "",
  logo_url: "",
  support_url: "",
};
const blankOffer: Offer = {
  id: 0,
  owner_id: 0,
  name: "",
  description: "",
  enabled: false,
  service_id: 0,
  amount: 0,
  currency: "IRT",
  duration_days: 30,
  traffic_bytes: 0,
  cost_amount: 0,
};
function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="label">
      {label}
      {children}
    </label>
  );
}
function Currency({
  value,
  onChange,
}: {
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <Select
      className="input"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      {["IRT", "IRR", "USD", "EUR", "XTR", "USDT"].map((v) => (
        <option key={v} value={v}>
          {v === "IRT"
            ? t("Toman", "تومان")
            : v === "IRR"
              ? t("Rial", "ریال")
              : v === "XTR"
                ? t("Telegram Stars", "استارز تلگرام")
                : v === "USDT"
                  ? "USDT"
                  : `${v} (${t("cents", "سنت")})`}
        </option>
      ))}
    </Select>
  );
}
export function CommerceWorkspace() {
  useLocale();
  const user = useSession(),
    editable = can(user, "commerce.write"),
    admin = !!user?.is_superuser;
  const [tab, setTab] = useState<Tab>("overview"),
    [owner, setOwner] = useState(user?.id || 0),
    [users, setUsers] = useState<Pick<AdminUser, "id" | "username">[]>([]);
  const [offers, setOffers] = useState<Offer[]>([]),
    [orders, setOrders] = useState<Order[]>([]),
    [wallets, setWallets] = useState<Wallet[]>([]),
    [ledger, setLedger] = useState<LedgerEntry[]>([]),
    [brand, setBrand] = useState<StoreBrand>(emptyStore),
    [reports, setReports] = useState<Reports>({
      summary: [],
      by_offer: [],
      by_node: [],
    });
  const [services, setServices] = useState<{ id: number; name: string }[]>([]);
  const [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const [editor, setEditor] = useState<Offer | null>(null),
    [dialog, setDialog] = useState<
      "adjust" | "credit" | "topup" | "purchase" | "cost" | null
    >(null);
  const [currency, setCurrency] = useState("IRT"),
    [amount, setAmount] = useState(""),
    [note, setNote] = useState(""),
    [reference, setReference] = useState(""),
    [node, setNode] = useState("");
  const [purchaseOffer, setPurchaseOffer] = useState(0),
    [purchaseName, setPurchaseName] = useState(""),
    [purchaseKind, setPurchaseKind] = useState("purchase"),
    [customerToken, setCustomerToken] = useState("");
  const [query, setQuery] = useState(""),
    [status, setStatus] = useState("");
  const scope = `?owner=${owner}`;
  const load = useCallback(
    async (signal?: AbortSignal) => {
      setLoading(true);
      setError("");
      try {
        const [o, q, w, l, s, r] = await Promise.all([
          api<Offer[]>(`/commerce/offers?owner=${owner}`, { signal }),
          api<Order[]>(`/commerce/orders?owner=${owner}`, { signal }),
          api<Wallet[]>(`/commerce/wallets?owner=${owner}`, { signal }),
          api<LedgerEntry[]>(`/commerce/ledger?owner=${owner}`, { signal }),
          api<StoreBrand>(`/commerce/store?owner=${owner}`, { signal }),
          api<Reports>(`/commerce/reports?owner=${owner}`, { signal }),
        ]);
        if (!signal?.aborted) {
          setOffers(o);
          setOrders(q);
          setWallets(w);
          setLedger(l);
          setBrand(s);
          setReports(r);
        }
      } catch (e) {
        if (!signal?.aborted) setError(commerceError(e));
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [owner],
  );
  useEffect(() => {
    const c = new AbortController();
    void load(c.signal);
    return () => c.abort();
  }, [load]);
  useEffect(() => {
    const c = new AbortController();
    void api<{ id: number; name: string }[]>("/services", { signal: c.signal })
      .then(setServices)
      .catch((e) => {
        if (!c.signal.aborted) setError(commerceError(e));
      });
    if (admin) {
      void api<AdminUser[]>("/admin/users/", { signal: c.signal })
        .then(setUsers)
        .catch((e) => {
          if (!c.signal.aborted) setError(commerceError(e));
        });
    } else if (can(user, "resellers.read")) {
      void api<Pick<AdminUser, "id" | "username">[]>("/resellers", {
        signal: c.signal,
      })
        .then((rows) =>
          setUsers([{ id: user!.id, username: user!.username }, ...rows]),
        )
        .catch((e) => {
          if (!c.signal.aborted) setError(commerceError(e));
        });
    }
    return () => c.abort();
  }, [admin]);
  async function mutate(
    action: () => Promise<unknown>,
    success = t("Saved", "ذخیره شد"),
  ) {
    if (busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
      setNotice(success);
      await load();
    } catch (e) {
      setError(commerceError(e));
    } finally {
      setBusy(false);
    }
  }
  function open(kind: NonNullable<typeof dialog>) {
    setAmount("");
    setNote("");
    setReference(crypto.randomUUID());
    setNode("");
    setError("");
    setDialog(kind);
  }
  async function saveOffer(e: FormEvent) {
    e.preventDefault();
    if (!editor) return;
    const {
      id,
      name,
      description,
      enabled,
      service_id,
      amount,
      currency,
      duration_days,
      traffic_bytes,
      cost_amount,
    } = editor;
    const data = {
      name,
      description,
      enabled,
      service_id,
      amount,
      currency,
      duration_days,
      traffic_bytes,
      cost_amount,
      category: editor.category || "",
      image_url: editor.image_url || "",
      sort_order: editor.sort_order || 0,
    };
    await mutate(async () => {
      await api(`/commerce/offers${id ? `/${id}` : ""}${scope}`, {
        method: id ? "PATCH" : "POST",
        body: JSON.stringify(data),
      });
      setEditor(null);
    });
  }
  async function submit(e: FormEvent) {
    e.preventDefault();
    await mutate(async () => {
      if (dialog === "purchase") {
        const order = await api<Order>("/commerce/purchase", {
          method: "POST",
          body: JSON.stringify({
            offer_id: purchaseOffer,
            kind: purchaseKind,
            name: purchaseName,
            customer_token: tokenFromURL(customerToken),
            reference,
          }),
        });
        rememberOrder(order);
        setTab("orders");
      } else if (dialog === "topup") {
        const order = await api<Order>("/commerce/topup", {
          method: "POST",
          body: JSON.stringify({ amount: Number(amount), currency }),
        });
        rememberOrder(order);
        window.location.assign(
          `/shop?store=${order.owner_id}&order=${order.id}#token=${encodeURIComponent(order.token || "")}`,
        );
      } else {
        const endpoint =
          dialog === "credit"
            ? "credit-limit"
            : dialog === "cost"
              ? "costs"
              : "adjust";
        const data =
          dialog === "credit"
            ? { credit_limit: Number(amount), currency }
            : {
                amount: Number(amount),
                currency,
                note,
                reference,
                ...(dialog === "cost" && node ? { node_id: Number(node) } : {}),
              };
        await api(`/commerce/${endpoint}${scope}`, {
          method: "POST",
          body: JSON.stringify(data),
        });
      }
      setDialog(null);
    });
  }
  const tabs: [Tab, string][] = [
    ["overview", t("Revenue", "درآمد")],
    ["offers", t("Offers", "بسته‌های فروش")],
    ["orders", t("Orders", "سفارش‌ها")],
    ["receipts", t("Receipts", "رسیدها")],
    ["tickets", t("Support", "پشتیبانی")],
    ["wallet", t("Wallet & ledger", "کیف پول و تراکنش‌ها")],
    ["store", t("Storefront", "فروشگاه")],
    ...(can(user, "resellers.read")
      ? ([["team", t("Team", "زیرمجموعه‌ها")]] as [Tab, string][])
      : []),
    ...(admin
      ? [["gateway", t("Payment gateway", "درگاه پرداخت")] as [Tab, string]]
      : []),
  ];
  const visibleOrders = orders.filter(
    (o) =>
      (!status || o.state === status) &&
      `${o.id} ${o.snapshot.name || ""} ${o.snapshot.customer_name || ""}`
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <ShoppingBag className="h-5 w-5 text-emerald-500" />
          <h2 className="font-semibold">
            {t("Sales & billing", "فروش و حسابداری")}
          </h2>
        </div>
        <div className="flex gap-2 items-end">
          {(admin || can(user, "resellers.read")) && (
            <Field label={t("Seller", "فروشنده")}>
              <Select
                className="input"
                value={owner}
                disabled={busy}
                onChange={(e) => setOwner(Number(e.target.value))}
              >
                {users.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.username}
                  </option>
                ))}
              </Select>
            </Field>
          )}
          <Button
            variant="outline"
            disabled={loading || busy}
            onClick={() => void load()}
            aria-label={t("Refresh", "تازه‌سازی")}
          >
            <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
          </Button>
        </div>
      </div>
      <nav
        aria-label={t("Sales sections", "بخش‌های فروش")}
        className="flex flex-wrap gap-1 border-b border-[var(--border)]"
      >
        {tabs.map(([key, label]) => (
          <button
            type="button"
            key={key}
            aria-current={tab === key ? "page" : undefined}
            onClick={() => setTab(key)}
            className={`border-b-2 px-3 py-2.5 text-[12px] transition-colors ${tab === key ? "border-emerald-500 text-emerald-500" : "border-transparent text-[var(--muted)] hover:text-[var(--fg)]"}`}
          >
            {label}
          </button>
        ))}
      </nav>
      {error && (
        <div
          role="alert"
          className="rounded-lg border border-red-500/20 bg-red-500/5 p-3 text-[12px] text-red-500"
        >
          {error}
        </div>
      )}
      {notice && (
        <div
          role="status"
          className="rounded-lg border border-emerald-500/20 bg-emerald-500/5 p-3 text-[12px] text-emerald-500"
        >
          {notice}
        </div>
      )}
      {loading ? (
        <p role="status" className="py-10 text-center text-[var(--muted)]">
          {t("Loading…")}
        </p>
      ) : (
        <>
          {tab === "team" && (
            <Suspense fallback={<p role="status">{t("Loading…")}</p>}>
              <ResellerTeam
                owner={owner}
                onNavigate={(id, name) => {
                  setUsers((rows) =>
                    rows.some((r) => r.id === id)
                      ? rows
                      : [...rows, { id, username: name }],
                  );
                  setOwner(id);
                }}
              />
            </Suspense>
          )}
          {tab === "overview" && (
            <>
              <div className="flex flex-wrap justify-end gap-2">
                {admin && (
                  <Button
                    variant="outline"
                    disabled={busy}
                    onClick={() => open("cost")}
                  >
                    {t("Record cost", "ثبت هزینه")}
                  </Button>
                )}
                {editable && (
                  <Button
                    disabled={busy}
                    onClick={() => {
                      setPurchaseOffer(offers.find((o) => o.enabled)?.id || 0);
                      setPurchaseName("");
                      setPurchaseKind("purchase");
                      setCustomerToken("");
                      open("purchase");
                    }}
                  >
                    <WalletIcon className="h-4 w-4" />
                    {t("Buy with wallet", "خرید با کیف پول")}
                  </Button>
                )}
              </div>
              <div className="grid gap-4 lg:grid-cols-2">
                {reports.summary.map((r) => (
                  <Card key={r.currency}>
                    <CardHeader>
                      <CardTitle>{r.currency}</CardTitle>
                      <span className="text-[12px] text-[var(--muted)]">
                        {r.orders} {t("Orders", "سفارش")}
                      </span>
                    </CardHeader>
                    <CardContent>
                      <dl className="grid grid-cols-2 gap-4 text-[12px]">
                        {[
                          [t("Revenue", "درآمد"), r.revenue],
                          [t("Profit", "سود"), r.profit],
                          [t("Unit costs", "هزینهٔ بسته‌ها"), r.unit_cost],
                          [
                            t("Operating costs", "هزینه‌های عملیاتی"),
                            r.operating_cost,
                          ],
                        ].map(([label, value]) => (
                          <div key={label}>
                            <dt className="text-[var(--muted)]">{label}</dt>
                            <dd className="mt-1 font-semibold tabular-nums">
                              {amountLabel(value, r.currency)}
                            </dd>
                          </div>
                        ))}
                      </dl>
                    </CardContent>
                  </Card>
                ))}
              </div>
              {reports.summary.length === 0 && (
                <Empty
                  text={t(
                    "No paid orders in the last 30 days.",
                    "در ۳۰ روز گذشته سفارش پرداخت‌شده‌ای ثبت نشده.",
                  )}
                />
              )}
              <Card>
                <CardHeader>
                  <CardTitle>
                    {t("Sales by offer", "فروش به تفکیک بسته")}
                  </CardTitle>
                  <span className="text-[11px] text-[var(--muted)]">
                    {t("Last 30 days", "۳۰ روز گذشته")}
                  </span>
                </CardHeader>
                <CardContent>
                  <div className="space-y-3">
                    {reports.by_offer.map((r) => (
                      <div
                        className="flex justify-between gap-4 text-[12px]"
                        key={`${r.offer_id}:${r.currency}`}
                      >
                        <span>
                          {offers.find((o) => o.id === r.offer_id)?.name ||
                            `#${r.offer_id}`}{" "}
                          · {r.orders}
                        </span>
                        <span className="tabular-nums">
                          {amountLabel(r.revenue, r.currency)}
                        </span>
                      </div>
                    ))}
                  </div>
                </CardContent>
              </Card>
              <CommerceBreakdown reports={reports} admin={admin} />
            </>
          )}
          {tab === "offers" && (
            <>
              <div className="flex justify-end">
                {editable && (
                  <Button
                    onClick={() => {
                      setError("");
                      setEditor({
                        ...blankOffer,
                        service_id: services[0]?.id || 0,
                      });
                    }}
                  >
                    <Plus className="h-4 w-4" />
                    {t("New offer", "بستهٔ جدید")}
                  </Button>
                )}
              </div>
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {offers.map((o) => (
                  <Card key={o.id}>
                    <CardHeader>
                      <div className="space-y-1">
                        <CardTitle>{o.name}</CardTitle>
                        <span
                          className={`text-[11px] ${o.enabled ? "text-emerald-500" : "text-[var(--muted)]"}`}
                        >
                          {o.enabled
                            ? t("Published", "منتشرشده")
                            : t("Draft", "پیش‌نویس")}
                        </span>
                      </div>
                      <Package className="h-4 w-4 text-[var(--muted)]" />
                    </CardHeader>
                    <CardContent className="space-y-4">
                      <p className="text-xl font-semibold tabular-nums">
                        {amountLabel(o.amount, o.currency)}
                      </p>
                      <p className="text-[12px] text-[var(--muted)]">
                        {o.duration_days} {t("days", "روز")} ·{" "}
                        {o.traffic_bytes
                          ? `${(o.traffic_bytes / 1073741824).toLocaleString()} GiB`
                          : t("Unlimited traffic", "حجم نامحدود")}
                      </p>
                      <p className="text-[12px] break-words">{o.description}</p>
                      {editable && (
                        <Button
                          variant="outline"
                          onClick={() => {
                            setError("");
                            setEditor({ ...o });
                          }}
                        >
                          {t("Edit")}
                        </Button>
                      )}
                    </CardContent>
                  </Card>
                ))}
              </div>
              {!offers.length && (
                <Empty
                  text={t(
                    "Create an offer from an existing service to start selling.",
                    "برای شروع فروش، از یک سرویس موجود بسته بسازید.",
                  )}
                />
              )}
            </>
          )}
          {tab === "orders" && (
            <Card>
              <CardHeader>
                <CardTitle>{t("Orders", "سفارش‌ها")}</CardTitle>
                <div className="flex flex-wrap gap-2">
                  <Input
                    className="input"
                    aria-label={t("Search orders", "جستجوی سفارش")}
                    placeholder={t("Search", "جستجو")}
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                  />
                  <Select
                    className="input"
                    aria-label={t("Order status", "وضعیت سفارش")}
                    value={status}
                    onChange={(e) => setStatus(e.target.value)}
                  >
                    <option value="">
                      {t("All statuses", "همهٔ وضعیت‌ها")}
                    </option>
                    {["pending", "paid", "fulfilled", "cancelled"].map((v) => (
                      <option key={v} value={v}>
                        {orderStatus(v)}
                      </option>
                    ))}
                  </Select>
                </div>
              </CardHeader>
              <CardContent>
                <div className="overflow-x-auto">
                  <table className="w-full text-start text-[12px]">
                    <thead>
                      <tr className="border-b border-[var(--border)] text-[var(--muted)]">
                        {[
                          t("Order", "سفارش"),
                          t("Amount", "مبلغ"),
                          t("Status", "وضعیت"),
                          t("Date", "تاریخ"),
                          "",
                        ].map((v, i) => (
                          <th className="p-3 text-start" key={i}>
                            {v}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {visibleOrders.map((o) => (
                        <tr
                          key={o.id}
                          className="border-b border-[var(--border-subtle)]"
                        >
                          <td className="p-3">
                            <div>{o.snapshot.name || o.kind}</div>
                            <div className="mt-1 text-[10px] text-[var(--muted)] font-mono">
                              {o.id.slice(0, 12)}
                            </div>
                            {o.snapshot.customer_name && (
                              <div className="mt-1 text-[var(--muted)]">
                                {o.snapshot.customer_name}
                              </div>
                            )}
                          </td>
                          <td className="p-3 whitespace-nowrap tabular-nums">
                            {amountLabel(o.amount, o.currency)}
                          </td>
                          <td className="p-3">
                            <span
                              className={
                                o.state === "fulfilled"
                                  ? "text-emerald-500"
                                  : o.state === "paid"
                                    ? "text-amber-500"
                                    : "text-[var(--muted)]"
                              }
                            >
                              {orderStatus(o.state)}
                            </span>
                            {o.last_error && (
                              <p className="mt-1 max-w-64 text-[10px] text-amber-500">
                                {o.last_error}
                              </p>
                            )}
                          </td>
                          <td className="p-3 whitespace-nowrap">
                            {new Date(o.created_at).toLocaleString()}
                          </td>
                          <td className="p-3">
                            {editable && o.state === "paid" && (
                              <Button
                                disabled={busy}
                                variant="outline"
                                onClick={() =>
                                  void mutate(() =>
                                    api(
                                      `/commerce/orders/${o.id}/fulfill${scope}`,
                                      { method: "POST", body: "{}" },
                                    ),
                                  )
                                }
                              >
                                {t("Retry delivery", "تلاش مجدد تحویل")}
                              </Button>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                {!visibleOrders.length && (
                  <Empty text={t("No orders", "سفارشی وجود ندارد")} />
                )}
              </CardContent>
            </Card>
          )}
          {tab === "wallet" && (
            <>
              <div className="flex flex-wrap justify-end gap-2">
                {admin && (
                  <>
                    <Button variant="outline" onClick={() => open("credit")}>
                      {t("Credit limit", "سقف اعتبار")}
                    </Button>
                    <Button variant="outline" onClick={() => open("adjust")}>
                      {t("Record adjustment", "ثبت واریز یا تسویه")}
                    </Button>
                  </>
                )}
                {editable && !admin && owner === user?.id && (
                  <Button onClick={() => open("topup")}>
                    <Plus className="h-4 w-4" />
                    {t("Top up wallet", "شارژ کیف پول")}
                  </Button>
                )}
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                {wallets.map((w) => (
                  <Card key={w.currency}>
                    <CardContent>
                      <div className="flex justify-between">
                        <WalletIcon className="h-5 w-5 text-emerald-500" />
                        <span className="text-[12px] text-[var(--muted)]">
                          {w.currency}
                        </span>
                      </div>
                      <p
                        className={`mt-4 text-2xl font-semibold tabular-nums ${w.balance < 0 ? "text-amber-500" : ""}`}
                      >
                        {amountLabel(w.balance, w.currency)}
                      </p>
                      <p className="mt-2 text-[11px] text-[var(--muted)]">
                        {t("Available credit", "اعتبار قابل استفاده")}:{" "}
                        {amountLabel(w.balance + w.credit_limit, w.currency)}
                      </p>
                    </CardContent>
                  </Card>
                ))}
              </div>
              <Card>
                <CardHeader>
                  <CardTitle>
                    {t("Transaction ledger", "دفتر تراکنش‌ها")}
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="divide-y divide-[var(--border-subtle)]">
                    {ledger.map((l) => (
                      <div
                        key={l.id}
                        className="flex items-start gap-3 py-3 text-[12px]"
                      >
                        {l.amount > 0 ? (
                          <ArrowDownLeft className="h-4 w-4 shrink-0 text-emerald-500" />
                        ) : (
                          <ArrowUpRight className="h-4 w-4 shrink-0 text-amber-500" />
                        )}
                        <div className="min-w-0 flex-1">
                          <p className="break-words">{l.note}</p>
                          <p className="mt-1 text-[10px] text-[var(--muted)]">
                            {new Date(l.created_at).toLocaleString()}
                          </p>
                        </div>
                        <div className="text-end tabular-nums">
                          <p>{amountLabel(l.amount, l.currency)}</p>
                          <p className="mt-1 text-[10px] text-[var(--muted)]">
                            {t("Balance", "موجودی")}:{" "}
                            {amountLabel(l.balance_after, l.currency)}
                          </p>
                        </div>
                      </div>
                    ))}
                  </div>
                  {!ledger.length && (
                    <Empty
                      text={t("No transactions yet.", "هنوز تراکنشی ثبت نشده.")}
                    />
                  )}
                </CardContent>
              </Card>
            </>
          )}
          {tab === "store" && (
            <Card>
              <CardHeader>
                <CardTitle>{t("Storefront", "فروشگاه")}</CardTitle>
                {brand.url && (
                  <a
                    href={brand.url}
                    target="_blank"
                    rel="noreferrer"
                    className="flex items-center gap-2 text-[12px] text-emerald-500"
                  >
                    {t("Open store", "مشاهدهٔ فروشگاه")}
                    <ExternalLink className="h-3.5 w-3.5" />
                  </a>
                )}
              </CardHeader>
              <CardContent>
                <form
                  className="max-w-2xl space-y-4"
                  onSubmit={(e) => {
                    e.preventDefault();
                    void mutate(() =>
                      api(`/commerce/store${scope}`, {
                        method: "PATCH",
                        body: JSON.stringify({
                          enabled: brand.enabled,
                          name: brand.name,
                          description: brand.description,
                          logo_url: brand.logo_url,
                          support_url: brand.support_url,
                          subscription_branding: !!brand.subscription_branding,
                          subscription_origin: brand.subscription_origin || "",
                          telegram_sales: !!brand.telegram_sales,
                          terms_url: brand.terms_url || "",
                          bot: brand.bot || {},
                        }),
                      }),
                    );
                  }}
                >
                  <label className="flex gap-2 text-[12px]">
                    <input
                      type="checkbox"
                      checked={brand.enabled}
                      disabled={!editable || busy}
                      onChange={(e) =>
                        setBrand({ ...brand, enabled: e.target.checked })
                      }
                    />
                    {t("Enable storefront", "فعال‌سازی فروشگاه")}
                  </label>
                  <Field label={t("Store name", "نام فروشگاه")}>
                    <Input
                      className="input"
                      required
                      maxLength={96}
                      value={brand.name}
                      disabled={!editable || busy}
                      onChange={(e) =>
                        setBrand({ ...brand, name: e.target.value })
                      }
                    />
                  </Field>
                  <Field label={t("Description", "توضیحات")}>
                    <Textarea
                      className="input"
                      maxLength={1200}
                      value={brand.description}
                      disabled={!editable || busy}
                      onChange={(e) =>
                        setBrand({ ...brand, description: e.target.value })
                      }
                    />
                  </Field>
                  {(
                    [
                      ["logo_url", t("Logo URL", "نشانی لوگو")],
                      ["support_url", t("Support URL", "لینک پشتیبانی")],
                    ] as const
                  ).map(([key, label]) => (
                    <Field key={key} label={label}>
                      <Input
                        className="input"
                        type="url"
                        placeholder="https://"
                        value={brand[key]}
                        disabled={!editable || busy}
                        onChange={(e) =>
                          setBrand({ ...brand, [key]: e.target.value })
                        }
                      />
                    </Field>
                  ))}
                  {brand.url && (
                    <Field label={t("Store link", "لینک فروشگاه")}>
                      <Input
                        className="input"
                        readOnly
                        value={brand.url}
                        dir="ltr"
                        onFocus={(e) => e.target.select()}
                      />
                    </Field>
                  )}
                  <StoreDomain
                    key={owner}
                    owner={owner}
                    brand={brand}
                    editable={editable && !busy}
                    onChange={(patch) =>
                      setBrand((current) => ({ ...current, ...patch }))
                    }
                    onVerified={(result) =>
                      setBrand((current) => ({ ...current, ...result }))
                    }
                  />
                  <SalesPayments
                    brand={brand}
                    editable={editable && !busy}
                    onChange={(patch) =>
                      setBrand((current) => ({ ...current, ...patch }))
                    }
                  />
                  <StoreTelegram
                    services={services}
                    brand={brand}
                    editable={editable && !busy}
                    onChange={(patch) =>
                      setBrand((current) => ({ ...current, ...patch }))
                    }
                  />
                  {editable && (
                    <Button disabled={busy} type="submit">
                      {t("Save")}
                    </Button>
                  )}
                </form>
              </CardContent>
            </Card>
          )}
          {tab === "gateway" && admin && (
            <Card>
              <CardHeader>
                <CardTitle>{t("Payment gateway", "درگاه پرداخت")}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3 py-10">
                <p className="text-2xl font-semibold">Soon...</p>
                <p className="text-[13px] leading-7 text-[var(--muted)]">
                  {t(
                    "If you would like a payment gateway added, let us know through Telegram.",
                    "اگر درگاهی مد نظرتان هست که اضافه شود، از طریق تلگرام اطلاع دهید.",
                  )}
                </p>
              </CardContent>
            </Card>
          )}
          {(tab === "receipts" || tab === "tickets") && (
            <Suspense fallback={<p role="status">{t("Loading…")}</p>}>
              <SalesReview
                key={`${owner}:${tab}`}
                owner={owner}
                editable={editable && !busy}
                orders={orders}
                mode={tab}
                onReviewed={() => void load()}
              />
            </Suspense>
          )}
        </>
      )}
      {editor && (
        <Dialog
          label={t("Offer", "بستهٔ فروش")}
          size="lg"
          onClose={() => !busy && setEditor(null)}
        >
          <form className="space-y-4" onSubmit={saveOffer}>
            <h3>{t("Offer", "بستهٔ فروش")}</h3>
            <Field label={t("Name")}>
              <Input
                className="input"
                required
                maxLength={96}
                value={editor.name}
                onChange={(e) => setEditor({ ...editor, name: e.target.value })}
              />
            </Field>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label={t("Category", "دسته‌بندی")}>
                <Input
                  maxLength={96}
                  value={editor.category || ""}
                  onChange={(e) =>
                    setEditor({ ...editor, category: e.target.value })
                  }
                />
              </Field>
              <Field label={t("Display order", "ترتیب نمایش")}>
                <Input
                  type="number"
                  min={0}
                  max={10000}
                  value={editor.sort_order || 0}
                  onChange={(e) =>
                    setEditor({ ...editor, sort_order: Number(e.target.value) })
                  }
                />
              </Field>
            </div>
            <Field label={t("Product image URL", "نشانی تصویر بسته")}>
              <Input
                type="url"
                dir="ltr"
                placeholder="https://"
                value={editor.image_url || ""}
                onChange={(e) =>
                  setEditor({ ...editor, image_url: e.target.value })
                }
              />
            </Field>
            <Field label={t("Service", "سرویس")}>
              <Select
                className="input"
                value={editor.service_id}
                onChange={(e) =>
                  setEditor({ ...editor, service_id: Number(e.target.value) })
                }
              >
                <option value={0}>{t("Select service", "انتخاب سرویس")}</option>
                {services.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </Select>
            </Field>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label={t("Price", "قیمت")}>
                <Input
                  className="input"
                  type="number"
                  required
                  min={editor.currency === "USDT" ? 0.000001 : 1}
                  max={editor.currency === "USDT" ? 1e6 : 1e12}
                  step={editor.currency === "USDT" ? 0.000001 : 1}
                  value={
                    editor.currency === "USDT"
                      ? editor.amount / 1000000 || ""
                      : editor.amount || ""
                  }
                  onChange={(e) =>
                    setEditor({
                      ...editor,
                      amount: Math.round(
                        Number(e.target.value) *
                          (editor.currency === "USDT" ? 1000000 : 1),
                      ),
                    })
                  }
                />
              </Field>
              <Field label={t("Currency", "واحد پول")}>
                <Currency
                  value={editor.currency}
                  onChange={(currency) => setEditor({ ...editor, currency })}
                />
              </Field>
              <Field label={t("Duration (days)", "مدت (روز)")}>
                <Input
                  className="input"
                  type="number"
                  required
                  min={1}
                  max={3650}
                  value={editor.duration_days}
                  onChange={(e) =>
                    setEditor({
                      ...editor,
                      duration_days: Number(e.target.value),
                    })
                  }
                />
              </Field>
              <Field
                label={t(
                  "Traffic (GiB) · 0 = unlimited",
                  "حجم (گیگابایت) · صفر = نامحدود",
                )}
              >
                <Input
                  className="input"
                  type="number"
                  required
                  min={0}
                  max={1073741824}
                  step="0.001"
                  value={editor.traffic_bytes / 1073741824}
                  onChange={(e) =>
                    setEditor({
                      ...editor,
                      traffic_bytes: Math.round(
                        Number(e.target.value) * 1073741824,
                      ),
                    })
                  }
                />
              </Field>
              <Field label={t("Unit cost", "هزینهٔ تأمین هر بسته")}>
                <Input
                  className="input"
                  type="number"
                  min={0}
                  max={editor.currency === "USDT" ? 1e6 : 1e12}
                  step={editor.currency === "USDT" ? 0.000001 : 1}
                  value={
                    editor.currency === "USDT"
                      ? editor.cost_amount / 1000000
                      : editor.cost_amount
                  }
                  onChange={(e) =>
                    setEditor({
                      ...editor,
                      cost_amount: Math.round(
                        Number(e.target.value) *
                          (editor.currency === "USDT" ? 1000000 : 1),
                      ),
                    })
                  }
                />
              </Field>
            </div>
            <Field label={t("Description", "توضیحات")}>
              <Textarea
                className="input"
                maxLength={1200}
                value={editor.description}
                onChange={(e) =>
                  setEditor({ ...editor, description: e.target.value })
                }
              />
            </Field>
            <label className="flex gap-2 text-[12px]">
              <input
                type="checkbox"
                checked={editor.enabled}
                onChange={(e) =>
                  setEditor({ ...editor, enabled: e.target.checked })
                }
              />
              {t("Publish offer", "انتشار بسته")}
            </label>
            {error && (
              <p role="alert" className="text-[12px] text-red-500">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setEditor(null)}
              >
                {t("Cancel")}
              </Button>
              <Button type="submit" disabled={busy}>
                {t("Save")}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
      {dialog && (
        <Dialog
          label={
            dialog === "purchase"
              ? t("Buy with wallet", "خرید با کیف پول")
              : t("Wallet & billing", "کیف پول و حسابداری")
          }
          onClose={() => !busy && setDialog(null)}
        >
          <form className="space-y-4" onSubmit={submit}>
            <h3>
              {dialog === "purchase"
                ? t("Buy with wallet", "خرید با کیف پول")
                : dialog === "cost"
                  ? t("Record cost", "ثبت هزینه")
                  : dialog === "credit"
                    ? t("Credit limit", "سقف اعتبار")
                    : dialog === "adjust"
                      ? t("Record adjustment", "ثبت واریز یا تسویه")
                      : t("Top up wallet", "شارژ کیف پول")}
            </h3>
            {dialog === "purchase" ? (
              <>
                <Field label={t("Offer", "بستهٔ فروش")}>
                  <Select
                    className="input"
                    value={purchaseOffer}
                    onChange={(e) => setPurchaseOffer(Number(e.target.value))}
                  >
                    {offers
                      .filter((o) => o.enabled)
                      .map((o) => (
                        <option key={o.id} value={o.id}>
                          {o.name} · {amountLabel(o.amount, o.currency)}
                        </option>
                      ))}
                  </Select>
                </Field>
                <Field label={t("Action", "عملیات")}>
                  <Select
                    className="input"
                    value={purchaseKind}
                    onChange={(e) => setPurchaseKind(e.target.value)}
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
                </Field>
                {purchaseKind === "purchase" ? (
                  <Field label={t("Customer name", "نام مشتری")}>
                    <Input
                      className="input"
                      required
                      maxLength={80}
                      value={purchaseName}
                      onChange={(e) => setPurchaseName(e.target.value)}
                    />
                  </Field>
                ) : (
                  <Field
                    label={t("Subscription URL or token", "لینک یا شناسهٔ ساب")}
                  >
                    <Input
                      className="input"
                      type="password"
                      autoComplete="off"
                      required
                      value={customerToken}
                      onChange={(e) => setCustomerToken(e.target.value)}
                    />
                  </Field>
                )}
                <p className="text-[11px] text-[var(--muted)]">
                  {t(
                    "Renewals add time and traffic without changing the subscription link or resetting usage.",
                    "تمدید، زمان و حجم را اضافه می‌کند؛ لینک ساب و مصرف قبلی حفظ می‌شوند.",
                  )}
                </p>
              </>
            ) : (
              <>
                <Field
                  label={
                    dialog === "credit"
                      ? t("Credit limit", "سقف اعتبار")
                      : t("Amount", "مبلغ")
                  }
                >
                  <Input
                    className="input"
                    type="number"
                    required
                    step={1}
                    min={
                      dialog === "adjust" ? -1e12 : dialog === "credit" ? 0 : 1
                    }
                    max={1e12}
                    value={amount}
                    onChange={(e) => setAmount(e.target.value)}
                  />
                </Field>
                <Field label={t("Currency", "واحد پول")}>
                  <Currency value={currency} onChange={setCurrency} />
                </Field>
                {["adjust", "cost"].includes(dialog) && (
                  <>
                    <Field label={t("Note", "شرح تراکنش")}>
                      <Input
                        className="input"
                        required
                        maxLength={500}
                        value={note}
                        onChange={(e) => setNote(e.target.value)}
                      />
                    </Field>
                    <Field label={t("Reference", "شناسهٔ پیگیری")}>
                      <Input
                        className="input"
                        required
                        minLength={12}
                        maxLength={160}
                        value={reference}
                        onChange={(e) => setReference(e.target.value)}
                      />
                    </Field>
                  </>
                )}
                {dialog === "adjust" && (
                  <p className="text-[11px] text-[var(--muted)]">
                    {t(
                      "Positive amounts add credit. Negative amounts record a debit or settlement.",
                      "مبلغ مثبت اعتبار اضافه می‌کند؛ مبلغ منفی برداشت یا تسویه را ثبت می‌کند.",
                    )}
                  </p>
                )}
                {dialog === "cost" && (
                  <Field
                    label={t("Node ID (optional)", "شناسهٔ نود (اختیاری)")}
                  >
                    <Input
                      className="input"
                      type="number"
                      min={1}
                      value={node}
                      onChange={(e) => setNode(e.target.value)}
                    />
                  </Field>
                )}
              </>
            )}
            {error && (
              <p role="alert" className="text-[12px] text-red-500">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setDialog(null)}
              >
                {t("Cancel")}
              </Button>
              <Button
                type="submit"
                disabled={busy || (dialog === "purchase" && !purchaseOffer)}
              >
                {t(
                  busy ? "Saving…" : "Continue",
                  busy ? "در حال انجام…" : "ادامه",
                )}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
    </div>
  );
}
function Empty({ text }: { text: string }) {
  return (
    <p className="py-10 text-center text-[12px] text-[var(--muted)]">{text}</p>
  );
}
