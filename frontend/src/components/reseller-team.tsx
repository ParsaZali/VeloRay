import { useCallback, useEffect, useState, type FormEvent } from "react";
import { ArrowDownRight, Copy, Plus, Users, Wallet } from "lucide-react";
import { api, type ClientInboundOption } from "@/lib/api";
import { can, useSession } from "@/lib/session";
import { amountLabel } from "@/lib/commerce";
import { t, useLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/controls";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog } from "@/components/common/dialog";

type Reseller = {
  id: number;
  username: string;
  email: string;
  parent_id: number;
  is_active: boolean;
  allowed_inbounds: number[];
  max_clients: number;
  traffic_budget_bytes: number;
  can_delegate: boolean;
  max_subresellers: number;
  clients_count: number;
  allocated_traffic_bytes: number;
  reserved_clients: number;
  reserved_traffic_bytes: number;
  may_delegate: boolean;
  unrestricted_inbounds: boolean;
};
type Editor = Omit<Reseller, "id"> & { id?: number; password: string };
const blank: Editor = {
  username: "",
  email: "",
  parent_id: 0,
  is_active: true,
  allowed_inbounds: [],
  max_clients: 0,
  traffic_budget_bytes: 0,
  can_delegate: false,
  max_subresellers: 0,
  clients_count: 0,
  allocated_traffic_bytes: 0,
  reserved_clients: 0,
  reserved_traffic_bytes: 0,
  may_delegate: false,
  unrestricted_inbounds: false,
  password: "",
};
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));
const volume = (n: number, unlimited = true) =>
  n === 0 && unlimited
    ? t("Unlimited", "نامحدود")
    : (n / 1073741824).toLocaleString(undefined, { maximumFractionDigits: 2 }) +
      " GiB";
export function ResellerTeam({
  owner,
  onNavigate,
}: {
  owner: number;
  onNavigate: (id: number, name: string) => void;
}) {
  useLocale();
  const session = useSession();
  const [rows, setRows] = useState<Reseller[]>([]),
    [policy, setPolicy] = useState<Reseller | null>(null),
    [inbounds, setInbounds] = useState<ClientInboundOption[]>([]),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [editor, setEditor] = useState<Editor | null>(null),
    [credential, setCredential] = useState<{
      username: string;
      password: string;
    } | null>(null);
  const [transfer, setTransfer] = useState<Reseller | null>(null),
    [amount, setAmount] = useState(""),
    [currency, setCurrency] = useState("IRT"),
    [note, setNote] = useState(""),
    [reference, setReference] = useState("");
  const editable = can(session, "resellers.write") && !!policy?.may_delegate;
  const load = useCallback(
    async (signal?: AbortSignal) => {
      setLoading(true);
      try {
        const [children, p, assigned] = await Promise.all([
          api<Reseller[]>(`/resellers?owner=${owner}`, { signal }),
          api<Reseller>(`/resellers/policy?owner=${owner}`, { signal }),
          api<ClientInboundOption[]>("/clients/inbound-options", { signal }),
        ]);
        if (!signal?.aborted) {
          setRows(children);
          setPolicy(p);
          setInbounds(assigned);
          setError("");
        }
      } catch (e) {
        if (!signal?.aborted) setError(message(e));
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
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!editor || busy) return;
    setBusy(true);
    setError("");
    try {
      const {
        username,
        email,
        is_active,
        allowed_inbounds,
        max_clients,
        traffic_budget_bytes,
        can_delegate,
        max_subresellers,
        password,
      } = editor;
      const out = await api<Reseller & { generated_password?: string }>(
        `/resellers${editor.id ? `/${editor.id}` : ""}?owner=${owner}`,
        {
          method: editor.id ? "PATCH" : "POST",
          body: JSON.stringify({
            username,
            email,
            is_active,
            allowed_inbounds,
            max_clients,
            traffic_budget_bytes,
            can_delegate,
            max_subresellers,
            ...(password ? { password } : {}),
          }),
        },
      );
      setEditor(null);
      if (out.generated_password)
        setCredential({
          username: out.username,
          password: out.generated_password,
        });
      await load();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function fund(e: FormEvent) {
    e.preventDefault();
    if (!transfer || busy) return;
    setBusy(true);
    setError("");
    try {
      await api("/resellers/transfer", {
        method: "POST",
        body: JSON.stringify({
          user_id: transfer.id,
          amount: Number(amount),
          currency,
          note,
          reference,
        }),
      });
      setTransfer(null);
      await load();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap justify-between gap-3">
        <div className="flex items-center gap-2">
          <Users className="h-5 w-5 text-emerald-500" />
          <h3 className="font-semibold">
            {t("Subresellers", "ریسلرهای زیرمجموعه")}
            {policy && (
              <span className="ms-2 text-[12px] font-normal text-[var(--muted)]">
                {policy.username}
              </span>
            )}
          </h3>
        </div>
        {editable && (
          <Button
            onClick={() => {
              setError("");
              setEditor({
                ...blank,
                max_clients: policy!.max_clients ? 1 : 0,
                traffic_budget_bytes: policy!.traffic_budget_bytes
                  ? 1073741824
                  : 0,
              });
            }}
          >
            <Plus className="h-4 w-4" />
            {t("Add subreseller", "افزودن ریسلر زیرمجموعه")}
          </Button>
        )}
      </div>
      {error && (
        <p
          role="alert"
          className="rounded-lg border border-red-500/20 bg-red-500/5 p-3 text-[12px] text-red-500"
        >
          {error}
        </p>
      )}
      {policy && (
        <Card>
          <CardContent>
            <dl className="grid gap-4 text-[12px] sm:grid-cols-3">
              <div>
                <dt className="text-[var(--muted)]">
                  {t("Direct customers", "مشتریان مستقیم")}
                </dt>
                <dd className="mt-1 font-semibold">
                  {policy.clients_count} /{" "}
                  {policy.max_clients || t("Unlimited", "نامحدود")}
                </dd>
              </div>
              <div>
                <dt className="text-[var(--muted)]">
                  {t("Reserved for children", "رزرو برای زیرمجموعه‌ها")}
                </dt>
                <dd className="mt-1 font-semibold">
                  {policy.reserved_clients} ·{" "}
                  {volume(policy.reserved_traffic_bytes, false)}
                </dd>
              </div>
              <div>
                <dt className="text-[var(--muted)]">
                  {t("Traffic budget", "بودجهٔ ترافیک")}
                </dt>
                <dd className="mt-1 font-semibold">
                  {volume(policy.traffic_budget_bytes)}
                </dd>
              </div>
            </dl>
          </CardContent>
        </Card>
      )}
      {loading ? (
        <p role="status">{t("Loading…")}</p>
      ) : rows.length ? (
        <div className="grid gap-4 lg:grid-cols-2">
          {rows.map((r) => (
            <Card key={r.id}>
              <CardHeader>
                <div>
                  <CardTitle>{r.username}</CardTitle>
                  <p
                    className={`mt-1 text-[11px] ${r.is_active ? "text-emerald-500" : "text-[var(--muted)]"}`}
                  >
                    {r.is_active
                      ? t("Login enabled", "ورود فعال")
                      : t("Login disabled", "ورود غیرفعال")}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  aria-label={`${t("Open branch", "مشاهدهٔ زیرمجموعه‌ها")} ${r.username}`}
                  onClick={() => onNavigate(r.id, r.username)}
                >
                  <ArrowDownRight className="h-4 w-4" />
                </Button>
              </CardHeader>
              <CardContent className="space-y-4">
                <dl className="grid grid-cols-2 gap-3 text-[12px]">
                  <div>
                    <dt className="text-[var(--muted)]">{t("Customers")}</dt>
                    <dd className="mt-1">
                      {r.clients_count} /{" "}
                      {r.max_clients || t("Unlimited", "نامحدود")}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-[var(--muted)]">
                      {t("Traffic budget", "بودجهٔ ترافیک")}
                    </dt>
                    <dd className="mt-1">{volume(r.traffic_budget_bytes)}</dd>
                  </div>
                </dl>
                <div className="flex flex-wrap gap-2">
                  {editable && (
                    <Button
                      variant="outline"
                      onClick={() => {
                        setError("");
                        setEditor({ ...r, password: "" });
                      }}
                    >
                      {t("Edit")}
                    </Button>
                  )}
                  {editable && owner === session?.id && (
                    <Button
                      variant="outline"
                      onClick={() => {
                        setError("");
                        setAmount("");
                        setNote("");
                        setReference(crypto.randomUUID());
                        setTransfer(r);
                      }}
                    >
                      <Wallet className="h-4 w-4" />
                      {t("Transfer credit", "انتقال اعتبار")}
                    </Button>
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <p className="py-10 text-center text-[12px] text-[var(--muted)]">
          {t("No subresellers yet.", "هنوز ریسلر زیرمجموعه‌ای ثبت نشده.")}
        </p>
      )}
      {editor && (
        <Dialog
          label={t("Subreseller", "ریسلر زیرمجموعه")}
          size="lg"
          onClose={() => !busy && setEditor(null)}
        >
          <form className="space-y-4" onSubmit={save}>
            <h3>{t("Subreseller", "ریسلر زیرمجموعه")}</h3>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="label">
                {t("Username")}
                <Input
                  className="input"
                  required
                  maxLength={150}
                  value={editor.username}
                  onChange={(e) =>
                    setEditor({ ...editor, username: e.target.value })
                  }
                />
              </label>
              <label className="label">
                {t("Email")}
                <Input
                  className="input"
                  type="email"
                  value={editor.email}
                  onChange={(e) =>
                    setEditor({ ...editor, email: e.target.value })
                  }
                />
              </label>
              <label className="label">
                {t("Password")}
                <Input
                  className="input"
                  type="password"
                  autoComplete="new-password"
                  minLength={12}
                  value={editor.password}
                  placeholder={
                    editor.id
                      ? t("Empty = keep current", "خالی = حفظ رمز فعلی")
                      : t("Empty = generate securely", "خالی = تولید خودکار")
                  }
                  onChange={(e) =>
                    setEditor({ ...editor, password: e.target.value })
                  }
                />
              </label>
              <label className="label">
                {t("Client limit", "سقف مشتری")}
                <Input
                  className="input"
                  type="number"
                  required
                  min={0}
                  value={editor.max_clients}
                  onChange={(e) =>
                    setEditor({
                      ...editor,
                      max_clients: Number(e.target.value),
                    })
                  }
                />
              </label>
              <label className="label">
                {t("Traffic budget (GiB)", "بودجهٔ ترافیک (گیگابایت)")}
                <Input
                  className="input"
                  type="number"
                  required
                  min={0}
                  step="0.001"
                  value={editor.traffic_budget_bytes / 1073741824}
                  onChange={(e) =>
                    setEditor({
                      ...editor,
                      traffic_budget_bytes: Math.round(
                        Number(e.target.value) * 1073741824,
                      ),
                    })
                  }
                />
              </label>
              <label className="label">
                {t("Direct subreseller limit", "سقف ریسلر مستقیم")}
                <Input
                  className="input"
                  type="number"
                  min={0}
                  max={100}
                  value={editor.max_subresellers}
                  onChange={(e) =>
                    setEditor({
                      ...editor,
                      max_subresellers: Number(e.target.value),
                    })
                  }
                />
              </label>
            </div>
            <label className="flex gap-2 text-[12px]">
              <input
                type="checkbox"
                checked={editor.is_active}
                onChange={(e) =>
                  setEditor({ ...editor, is_active: e.target.checked })
                }
              />
              {t("Login enabled", "ورود فعال")}
            </label>
            <label className="flex gap-2 text-[12px]">
              <input
                type="checkbox"
                checked={editor.can_delegate}
                onChange={(e) =>
                  setEditor({ ...editor, can_delegate: e.target.checked })
                }
              />
              {t("Allow subresellers", "اجازهٔ ساخت ریسلر زیرمجموعه")}
            </label>
            <fieldset className="space-y-2">
              <legend className="mb-2 text-[12px] font-medium">
                {t("Assigned inbounds")}
              </legend>
              <div className="max-h-44 space-y-2 overflow-y-auto rounded-lg border border-[var(--border)] p-3">
                {inbounds
                  .filter(
                    (i) =>
                      policy?.unrestricted_inbounds ||
                      policy?.allowed_inbounds?.includes(i.id),
                  )
                  .map((i) => (
                    <label className="flex gap-2 text-[12px]" key={i.id}>
                      <input
                        type="checkbox"
                        checked={editor.allowed_inbounds.includes(i.id)}
                        onChange={(e) =>
                          setEditor({
                            ...editor,
                            allowed_inbounds: e.target.checked
                              ? [...editor.allowed_inbounds, i.id]
                              : editor.allowed_inbounds.filter(
                                  (id) => id !== i.id,
                                ),
                          })
                        }
                      />
                      {i.name} · {i.node_name}
                    </label>
                  ))}
              </div>
            </fieldset>
            <p className="text-[11px] text-[var(--muted)]">
              {t(
                "Child quotas reserve capacity from the parent. Limited parents cannot assign unlimited quotas.",
                "سهمیهٔ زیرمجموعه از ظرفیت والد رزرو می‌شود. والد محدود نمی‌تواند سهمیهٔ نامحدود واگذار کند.",
              )}
            </p>
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
              <Button disabled={busy} type="submit">
                {t(busy ? "Saving…" : "Save")}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
      {credential && (
        <Dialog
          label={t("Reseller credentials", "اطلاعات ورود ریسلر")}
          onClose={() => setCredential(null)}
        >
          <div className="space-y-4">
            <h3>{t("Reseller credentials", "اطلاعات ورود ریسلر")}</h3>
            <label className="label">
              {t("Username")}
              <Input className="input" readOnly value={credential.username} />
            </label>
            <label className="label">
              {t("Password")}
              <Input
                className="input"
                readOnly
                type="password"
                value={credential.password}
              />
            </label>
            <p className="text-[11px] text-[var(--muted)]">
              {t(
                "Save this generated password. It is shown once.",
                "این رمز تولیدشده را ذخیره کنید؛ فقط همین بار نمایش داده می‌شود.",
              )}
            </p>
            <Button
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(credential.password);
                } catch {
                  setError(t("Copy failed", "کپی ناموفق بود"));
                }
              }}
            >
              <Copy className="h-4 w-4" />
              {t("Copy password", "کپی رمز")}
            </Button>
          </div>
        </Dialog>
      )}
      {transfer && (
        <Dialog
          label={t("Transfer credit", "انتقال اعتبار")}
          onClose={() => !busy && setTransfer(null)}
        >
          <form className="space-y-4" onSubmit={fund}>
            <h3>
              {t("Transfer credit", "انتقال اعتبار")} · {transfer.username}
            </h3>
            <label className="label">
              {t("Amount", "مبلغ")}
              <Input
                className="input"
                type="number"
                required
                min={1}
                max={1e12}
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
              />
            </label>
            <label className="label">
              {t("Currency", "واحد پول")}
              <Select
                className="input"
                value={currency}
                onChange={(e) => setCurrency(e.target.value)}
              >
                {["IRT", "IRR", "USD", "EUR"].map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </Select>
            </label>
            <label className="label">
              {t("Note", "شرح تراکنش")}
              <Input
                className="input"
                maxLength={400}
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </label>
            <p className="text-[11px] text-[var(--muted)]">
              {t(
                "Funds are debited from your wallet and credited to this direct subreseller in one transaction.",
                "مبلغ در یک تراکنش از کیف پول شما کم و به کیف پول این زیرمجموعه اضافه می‌شود.",
              )}
              {amount && <> · {amountLabel(Number(amount), currency)}</>}
            </p>
            {error && (
              <p role="alert" className="text-[12px] text-red-500">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setTransfer(null)}
              >
                {t("Cancel")}
              </Button>
              <Button type="submit" disabled={busy}>
                {t("Transfer credit", "انتقال اعتبار")}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
    </div>
  );
}
