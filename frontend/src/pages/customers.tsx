import { useEffect, useState, type FormEvent } from "react";
import { api, type InboundItem } from "@/lib/api";
import { can, useSession } from "@/lib/session";
import { t, useLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/controls";
import { Dialog } from "@/components/common/dialog";
import { PageHeader } from "@/components/common/page-header";
import { CustomerFreeze } from "@/components/customer-freeze";
import { ImpactSummary, type ChangeImpact } from "@/components/change-impact";
import { lazy, Suspense } from "react";
const CommerceWorkspace = lazy(() =>
  import("./commerce").then((module) => ({
    default: module.CommerceWorkspace,
  })),
);

type Policy = {
  renewal_period: string;
  renewal_every: number;
  renewal_timezone: string;
  traffic_limit_bytes: number;
  traffic_multiplier: number;
  hwid_limit: number;
  device_limit: number;
  start_after_first_use: boolean;
  duration_days: number;
  renewal_interval_days: number;
};
type Item = Partial<Policy> & {
  frozen?: boolean;
  freeze_until?: string;
  id?: number;
  name: string;
  note: string;
  inbound_ids: number[];
  service_ids?: number[];
  plan_id?: number;
  enabled?: boolean;
  expires_at?: string | null;
  defaults?: Partial<Policy>;
  subscription_url?: string;
  used_traffic_bytes?: number;
  connections?: unknown[];
};
const policy: Policy = {
  renewal_period: "none",
  renewal_every: 1,
  renewal_timezone: "UTC",
  traffic_limit_bytes: 0,
  traffic_multiplier: 1,
  hwid_limit: 0,
  device_limit: 0,
  start_after_first_use: false,
  duration_days: 30,
  renewal_interval_days: 0,
};
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));
const flip = (ids: number[], id: number) =>
  ids.includes(id) ? ids.filter((i) => i !== id) : [...ids, id];
export function CustomersPage() {
  return <Workspace kind="customers" />;
}
export function ServicesPage() {
  const session = useSession();
  useLocale();
  const [section, setSection] = useState("services");
  return (
    <div className="space-y-5">
      {can(session, "commerce.read") && (
        <div
          className="inline-flex gap-1 rounded-lg border border-[var(--border)] bg-[var(--card)] p-1"
          aria-label={t("Service workspace", "فضای سرویس‌ها")}
        >
          {[
            ["services", t("Services and plans")],
            ["sales", t("Sales & billing", "فروش و حسابداری")],
          ].map(([value, label]) => (
            <button
              type="button"
              aria-pressed={section === value}
              key={value}
              onClick={() => setSection(value)}
              className={`rounded-md px-4 py-2 text-[12px] transition-colors ${section === value ? "bg-emerald-500/10 text-emerald-500" : "text-[var(--muted)]"}`}
            >
              {label}
            </button>
          ))}
        </div>
      )}
      {section === "sales" ? (
        <Suspense fallback={<p role="status">{t("Loading…")}</p>}>
          <CommerceWorkspace />
        </Suspense>
      ) : (
        <Workspace kind="services" />
      )}
    </div>
  );
}
function Workspace({ kind }: { kind: "customers" | "services" }) {
  useLocale();
  const session = useSession(),
    services = kind === "services",
    editable = services
      ? !!session?.is_superuser
      : can(session, "clients.write");
  const [rows, setRows] = useState<Item[]>([]),
    [plans, setPlans] = useState<Item[]>([]),
    [inbounds, setInbounds] = useState<InboundItem[]>([]),
    [editor, setEditor] = useState<Item | null>(null),
    [deleting, setDeleting] = useState<Item | null>(null),
    [rotating, setRotating] = useState<Item | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true),
    [query, setQuery] = useState(""),
    [serviceFilter, setServiceFilter] = useState(0),
    [selected, setSelected] = useState<number[]>([]),
    [bulkOpen, setBulkOpen] = useState(false),
    [bulkAction, setBulkAction] = useState("disable"),
    [quotaPatch, setQuotaPatch] = useState(false),
    [quota, setQuota] = useState("0"),
    [expiryPatch, setExpiryPatch] = useState(false),
    [expiry, setExpiry] = useState(""),
    [accessPatch, setAccessPatch] = useState(false),
    [bulkServices, setBulkServices] = useState<number[]>([]);
  const [impact, setImpact] = useState<ChangeImpact | null>(null);
  useEffect(() => setImpact(null), [editor]);
  async function load(signal?: AbortSignal) {
    try {
      const [a, b, c] = await Promise.all([
        api<Item[]>(`/${kind}`, { signal }),
        services ? Promise.resolve([]) : api<Item[]>("/services", { signal }),
        api<InboundItem[]>("/inbounds", { signal }),
      ]);
      if (!signal?.aborted) {
        setRows(a);
        setSelected((ids) =>
          ids.filter((id) => a.some((row) => row.id === id)),
        );
        setPlans(b);
        setInbounds(c);
        setLoading(false);
      }
    } catch (e) {
      if (!signal?.aborted) {
        setError(message(e));
        setLoading(false);
      }
    }
  }
  useEffect(() => {
    const c = new AbortController();
    void load(c.signal);
    return () => c.abort();
  }, [kind]);
  function open(item?: Item) {
    setError("");
    setEditor(
      item
        ? structuredClone(item)
        : {
            name: "",
            note: "",
            inbound_ids: [],
            ...(services
              ? { defaults: { ...policy } }
              : {
                  ...policy,
                  enabled: true,
                  expires_at: null,
                  service_ids: [],
                  plan_id: 0,
                }),
          },
    );
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!editor) return;
    setBusy(true);
    setError("");
    try {
      const keys = services
        ? ["name", "note", "inbound_ids", "defaults"]
        : [
            "name",
            "note",
            "inbound_ids",
            "service_ids",
            "plan_id",
            "enabled",
            "expires_at",
            ...Object.keys(policy),
          ];
      const body = Object.fromEntries(
        Object.entries(editor).filter(([k]) => keys.includes(k)),
      );
      if (services && editor.id && !impact) {
        setImpact(
          await api(`/services/${editor.id}/impact`, {
            method: "POST",
            body: JSON.stringify(body),
          }),
        );
        return;
      }
      if (services && impact) body.expected_revision = impact.revision;
      await api(`/${kind}${editor.id ? `/${editor.id}` : ""}`, {
        method: editor.id ? "PATCH" : "POST",
        body: JSON.stringify(body),
      });
      setEditor(null);
      await load();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (!deleting) return;
    setBusy(true);
    try {
      await api(`/${kind}/${deleting.id}`, { method: "DELETE" });
      setDeleting(null);
      await load();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  const selectedPolicy = {
    ...policy,
    ...(services ? editor?.defaults : editor),
  };
  function changePolicy(key: keyof Policy, v: number | boolean | string) {
    setEditor(
      (d) =>
        d &&
        (services
          ? { ...d, defaults: { ...policy, ...d.defaults, [key]: v } }
          : { ...d, [key]: v }),
    );
  }
  const visibleRows = rows.filter(
    (i) =>
      `${i.name} ${i.note}`.toLowerCase().includes(query.toLowerCase()) &&
      (!serviceFilter || i.service_ids?.includes(serviceFilter)),
  );
  async function bulkSave(e: FormEvent) {
    e.preventDefault();
    if (!selected.length || selected.length > 200) return;
    setBusy(true);
    setError("");
    const patch = {
      ...(quotaPatch
        ? { traffic_limit_bytes: Math.round(Number(quota) * 1073741824) }
        : {}),
      ...(expiryPatch
        ? { expires_at: expiry ? new Date(expiry + "Z").toISOString() : null }
        : {}),
      ...(accessPatch ? { service_ids: bulkServices } : {}),
    };
    try {
      await api("/customers/bulk", {
        method: "POST",
        body: JSON.stringify({
          ids: selected,
          action: bulkAction,
          ...(bulkAction === "patch" ? { patch } : {}),
        }),
      });
      setBulkOpen(false);
      setSelected([]);
      await load();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-5">
      <PageHeader
        title={services ? "Services and plans" : "Customers"}
        description=""
        actions={
          editable ? (
            <Button onClick={() => open()}>
              {t(services ? "Add service" : "Add customer")}
            </Button>
          ) : undefined
        }
      />
      <p className="text-[12px] text-[var(--muted)]">
        {t(
          services
            ? "Services grant access to their inbounds. Plan defaults apply when selected for a customer."
            : "One account shares quota, expiry and subscription across inbounds and nodes.",
        )}
      </p>
      {error && !editor && !deleting && !rotating && !bulkOpen && (
        <p role="alert" className="text-red-500">
          {error}
        </p>
      )}
      <Input
        aria-label={t("Search")}
        placeholder={t("Search")}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {!services && (
        <div className="flex flex-wrap items-end gap-3">
          <label className="label min-w-48">
            {t("Filter by service")}
            <Select
              value={serviceFilter}
              onChange={(e) => setServiceFilter(+e.target.value)}
            >
              <option value={0}>{t("All services")}</option>
              {plans.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
          </label>
          {editable && (
            <>
              <Button
                variant="outline"
                disabled={!visibleRows.length}
                onClick={() =>
                  setSelected(visibleRows.slice(0, 200).map((i) => i.id!))
                }
              >
                {t("Select visible")}
              </Button>
              <Button
                variant="outline"
                disabled={!selected.length}
                onClick={() => setSelected([])}
              >
                {t("Clear selection")}
              </Button>
              <Button
                disabled={!selected.length}
                onClick={() => {
                  setError("");
                  setBulkOpen(true);
                }}
              >
                {t("Bulk actions")} ({selected.length})
              </Button>
              <span className="text-[11px] text-[var(--muted)]">
                {t(
                  "Select up to 200 customers. Selection includes hidden rows.",
                )}
              </span>
            </>
          )}
        </div>
      )}
      {loading ? (
        <p role="status">{t("Loading…")}</p>
      ) : (
        <div className="divide-y divide-[var(--border)] rounded-md border border-[var(--border)]">
          {visibleRows.map((item) => (
            <article
              key={item.id}
              className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center"
            >
              {!services && editable && (
                <input
                  type="checkbox"
                  aria-label={`${t("Select")} ${item.name}`}
                  checked={selected.includes(item.id!)}
                  disabled={
                    !selected.includes(item.id!) && selected.length >= 200
                  }
                  onChange={() => setSelected((ids) => flip(ids, item.id!))}
                />
              )}
              <div className="min-w-0 flex-1">
                <h3 className="break-words text-[13px] font-medium">
                  {item.name}
                </h3>
                <p className="break-words text-[12px] text-[var(--muted)]">
                  {item.note}
                </p>
                {!services && (
                  <p className="text-[11px] text-[var(--muted)]">
                    {item.connections?.length || 0} {t("Connections")} ·{" "}
                    {((item.used_traffic_bytes || 0) / 1073741824).toFixed(2)}{" "}
                    GiB /{" "}
                    {item.traffic_limit_bytes
                      ? (item.traffic_limit_bytes / 1073741824).toFixed(2) +
                        " GiB"
                      : t("Unlimited")}
                  </p>
                )}
              </div>
              <div className="flex flex-wrap gap-2">
                {!services && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={async () => {
                      try {
                        await navigator.clipboard.writeText(
                          item.subscription_url || "",
                        );
                        setError("");
                      } catch (e) {
                        setError(message(e));
                      }
                    }}
                  >
                    {t("Copy subscription")}
                  </Button>
                )}
                {!services && editable && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setError("");
                      setRotating(item);
                    }}
                  >
                    {t("Rotate subscription")}
                  </Button>
                )}
                {!services &&
                  editable &&
                  item.id &&
                  (item.frozen || (item.enabled && item.expires_at)) && (
                    <CustomerFreeze
                      id={item.id}
                      name={item.name}
                      frozen={item.frozen}
                      until={item.freeze_until}
                      onChanged={load}
                    />
                  )}
                {editable && (
                  <Button
                    size="sm"
                    variant="outline"
                    aria-label={`${t("Edit")} ${item.name}`}
                    onClick={() => open(item)}
                  >
                    {t("Edit")}
                  </Button>
                )}
                {(services ? editable : can(session, "clients.delete")) && (
                  <Button
                    size="sm"
                    variant="danger"
                    aria-label={`${t("Delete")} ${item.name}`}
                    onClick={() => {
                      setError("");
                      setDeleting(item);
                    }}
                  >
                    {t("Delete")}
                  </Button>
                )}
              </div>
            </article>
          ))}
          {visibleRows.length === 0 && (
            <p className="p-4 text-[12px]">{t("No records yet")}</p>
          )}
        </div>
      )}
      {bulkOpen && (
        <Dialog
          label={t("Bulk actions")}
          onClose={() => !busy && setBulkOpen(false)}
        >
          <form onSubmit={bulkSave} className="space-y-4">
            <h3>
              {t("Bulk actions")} · {selected.length}
            </h3>
            <p className="text-[12px]">
              {t(
                "This affects every selected customer and all their connections. Changes roll back together if deployment fails.",
              )}
            </p>
            <label className="label">
              {t("Action")}
              <Select
                value={bulkAction}
                onChange={(e) => setBulkAction(e.target.value)}
              >
                <option value="disable">{t("Disable")}</option>
                <option value="enable">{t("Enable")}</option>
                <option value="reset">{t("Reset usage")}</option>
                <option value="patch">{t("Edit quota and access")}</option>
                {can(session, "clients.delete") && (
                  <option value="delete">{t("Delete")}</option>
                )}
              </Select>
            </label>
            {bulkAction === "patch" && (
              <>
                <label className="flex gap-2 text-[12px]">
                  <input
                    type="checkbox"
                    checked={quotaPatch}
                    onChange={(e) => setQuotaPatch(e.target.checked)}
                  />
                  {t("Replace traffic quota")}
                </label>
                {quotaPatch && (
                  <label className="label">
                    {t("Traffic quota (GiB)")}
                    <Input
                      type="number"
                      required
                      min={0}
                      step="0.001"
                      value={quota}
                      onChange={(e) => setQuota(e.target.value)}
                    />
                  </label>
                )}
                <label className="flex gap-2 text-[12px]">
                  <input
                    type="checkbox"
                    checked={expiryPatch}
                    onChange={(e) => setExpiryPatch(e.target.checked)}
                  />
                  {t("Replace expiry")}
                </label>
                {expiryPatch && (
                  <label className="label">
                    {t("Expiry (UTC)")}
                    <Input
                      type="datetime-local"
                      value={expiry}
                      onChange={(e) => setExpiry(e.target.value)}
                    />
                    <span>{t("Empty expiry means no expiration.")}</span>
                  </label>
                )}
                <label className="flex gap-2 text-[12px]">
                  <input
                    type="checkbox"
                    checked={accessPatch}
                    onChange={(e) => setAccessPatch(e.target.checked)}
                  />
                  {t("Replace service access")}
                </label>
                {accessPatch && (
                  <fieldset className="space-y-2">
                    <legend>{t("Services and plans")}</legend>
                    <p className="text-[11px] text-[var(--muted)]">
                      {t(
                        "Direct inbounds and usage are preserved. Plan defaults are not reapplied.",
                      )}
                    </p>
                    {plans.map((p) => (
                      <label key={p.id} className="flex gap-2 text-[12px]">
                        <input
                          type="checkbox"
                          checked={bulkServices.includes(p.id!)}
                          onChange={() =>
                            setBulkServices((ids) => flip(ids, p.id!))
                          }
                        />
                        {p.name}
                      </label>
                    ))}
                  </fieldset>
                )}
              </>
            )}
            {bulkAction === "reset" && (
              <p className="text-[12px]">
                {t(
                  "Current usage will be reset. Lifetime traffic is preserved.",
                )}
              </p>
            )}
            {bulkAction === "delete" && (
              <p role="note" className="text-[12px] text-red-500">
                {t(
                  "Selected customers, their connections and subscription URLs will be deleted.",
                )}
              </p>
            )}
            {error && (
              <p role="alert" className="text-red-500">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setBulkOpen(false)}
              >
                {t("Cancel")}
              </Button>
              <Button
                type="submit"
                variant={bulkAction === "delete" ? "danger" : "default"}
                disabled={
                  busy ||
                  (bulkAction === "patch" &&
                    !quotaPatch &&
                    !expiryPatch &&
                    !accessPatch)
                }
              >
                {t(busy ? "Saving…" : "Apply to selected")}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
      {editor && (
        <Dialog
          label={t(services ? "Service" : "Customer")}
          size="lg"
          onClose={() => !busy && setEditor(null)}
        >
          <form onSubmit={save} className="space-y-4">
            <h3>{t(services ? "Service" : "Customer")}</h3>
            {services && impact && <ImpactSummary impact={impact} />}
            <label className="label">
              {t("Name")}
              <Input
                required
                maxLength={96}
                value={editor.name}
                onChange={(e) =>
                  setEditor((d) => d && { ...d, name: e.target.value })
                }
              />
            </label>
            <label className="label">
              {t("Note")}
              <Input
                value={editor.note}
                onChange={(e) =>
                  setEditor((d) => d && { ...d, note: e.target.value })
                }
              />
            </label>
            {!services && (
              <>
                <fieldset className="space-y-2">
                  <legend>{t("Services and plans")}</legend>
                  {plans.map((p) => (
                    <label key={p.id} className="flex gap-2 text-[12px]">
                      <input
                        type="checkbox"
                        checked={editor.service_ids?.includes(p.id!) || false}
                        onChange={() =>
                          setEditor(
                            (d) =>
                              d && {
                                ...d,
                                service_ids: flip(d.service_ids || [], p.id!),
                                plan_id: d.plan_id === p.id ? 0 : d.plan_id,
                              },
                          )
                        }
                      />
                      {p.name}
                    </label>
                  ))}
                </fieldset>
                <label className="label">
                  {t("Apply plan defaults")}
                  <Select
                    value={editor.plan_id || 0}
                    onChange={(e) => {
                      const id = +e.target.value,
                        p = plans.find((i) => i.id === id);
                      setEditor(
                        (d) => d && { ...d, ...p?.defaults, plan_id: id },
                      );
                    }}
                  >
                    <option value={0}>{t("Custom")}</option>
                    {plans
                      .filter((p) => editor.service_ids?.includes(p.id!))
                      .map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name}
                        </option>
                      ))}
                  </Select>
                </label>
                <label className="flex gap-2 text-[12px]">
                  <input
                    type="checkbox"
                    checked={editor.enabled || false}
                    onChange={(e) =>
                      setEditor((d) => d && { ...d, enabled: e.target.checked })
                    }
                  />
                  {t("Enabled")}
                </label>
                <label className="label">
                  {t("Expiry (UTC)")}
                  <Input
                    type="datetime-local"
                    value={editor.expires_at?.slice(0, 16) || ""}
                    onChange={(e) =>
                      setEditor(
                        (d) =>
                          d && {
                            ...d,
                            expires_at: e.target.value
                              ? new Date(e.target.value + "Z").toISOString()
                              : null,
                          },
                      )
                    }
                  />
                </label>
              </>
            )}
            <fieldset className="space-y-2">
              <legend>{t("Inbounds")}</legend>
              <div className="max-h-56 overflow-y-auto space-y-2 rounded-md border border-[var(--border)] p-3">
                {inbounds.map((i) => (
                  <label key={i.id} className="flex gap-2 text-[12px]">
                    <input
                      type="checkbox"
                      checked={editor.inbound_ids.includes(i.id)}
                      onChange={() =>
                        setEditor(
                          (d) =>
                            d && {
                              ...d,
                              inbound_ids: flip(d.inbound_ids, i.id),
                            },
                        )
                      }
                    />
                    <span className="break-words min-w-0">
                      {i.name} · {i.node_name} · {i.protocol}
                    </span>
                  </label>
                ))}
                {inbounds.length === 0 && <p>{t("No inbounds available")}</p>}
              </div>
            </fieldset>
            <fieldset className="space-y-3">
              <legend>{t("Quota and access")}</legend>
              <div className="grid gap-3 sm:grid-cols-3">
                <label className="label">
                  {t("Calendar renewal")}
                  <Select
                    value={selectedPolicy.renewal_period}
                    onChange={(e) =>
                      changePolicy("renewal_period", e.target.value)
                    }
                  >
                    {[
                      ["none", "Use day interval"],
                      ["hour", "Hourly"],
                      ["day", "Daily"],
                      ["week", "Weekly"],
                      ["month", "Monthly"],
                    ].map(([v, l]) => (
                      <option key={v} value={v}>
                        {t(l)}
                      </option>
                    ))}
                  </Select>
                </label>
                <label className="label">
                  {t("Every")}
                  <Input
                    type="number"
                    min={1}
                    max={3650}
                    disabled={selectedPolicy.renewal_period === "none"}
                    value={selectedPolicy.renewal_every}
                    onChange={(e) =>
                      changePolicy("renewal_every", +e.target.value)
                    }
                  />
                </label>
                <label className="label">
                  {t("Timezone")}
                  <Input
                    value={selectedPolicy.renewal_timezone}
                    onChange={(e) =>
                      changePolicy("renewal_timezone", e.target.value)
                    }
                    placeholder="Asia/Tehran"
                  />
                </label>
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="label">
                  {t("Traffic quota (GiB)")}
                  <Input
                    type="number"
                    min={0}
                    step="0.001"
                    value={selectedPolicy.traffic_limit_bytes / 1073741824}
                    onChange={(e) =>
                      changePolicy(
                        "traffic_limit_bytes",
                        Math.round(+e.target.value * 1073741824),
                      )
                    }
                  />
                </label>
                {(
                  [
                    [
                      "traffic_multiplier",
                      "Traffic multiplier",
                      -1000,
                      1000,
                      "0.001",
                    ],
                    ["hwid_limit", "HWID limit", -1, 1000, "1"],
                    ["device_limit", "Concurrent IP limit", 0, 10000, "1"],
                    [
                      "renewal_interval_days",
                      "Renewal interval (days)",
                      0,
                      3650,
                      "1",
                    ],
                    [
                      "duration_days",
                      "Duration after first use (days)",
                      1,
                      36500,
                      "1",
                    ],
                  ] as const
                ).map(([key, label, min, max, step]) => (
                  <label key={key} className="label">
                    {t(label)}
                    <Input
                      type="number"
                      min={min}
                      max={max}
                      step={step}
                      value={selectedPolicy[key]}
                      onChange={(e) => changePolicy(key, +e.target.value)}
                    />
                  </label>
                ))}
              </div>
              <label className="flex gap-2 text-[12px]">
                <input
                  type="checkbox"
                  checked={selectedPolicy.start_after_first_use}
                  onChange={(e) =>
                    changePolicy("start_after_first_use", e.target.checked)
                  }
                />
                {t("Start duration after first use")}
              </label>
              <p className="text-[11px] text-[var(--muted)]">
                {t(
                  "HWID: 0 inherits the global limit; -1 disables it. Zero quota or IP limit means unlimited.",
                )}
              </p>
            </fieldset>
            {error && (
              <p role="alert" className="text-red-500">
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
                {busy
                  ? t("Saving…")
                  : services && editor.id
                    ? t(
                        impact ? "Save & deploy" : "Review changes",
                        impact ? "ذخیره و اعمال" : "بررسی تغییرات",
                      )
                    : t("Save")}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
      {rotating && (
        <Dialog
          label={t("Rotate subscription")}
          onClose={() => !busy && setRotating(null)}
        >
          <div className="space-y-4">
            <h3>{rotating.name}</h3>
            <p className="text-[12px]">
              {t(
                "All existing subscription URLs will stop working. Share the new URL with this customer.",
              )}
            </p>
            {error && (
              <p role="alert" className="text-red-500">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setRotating(null)}
              >
                {t("Cancel")}
              </Button>
              <Button
                disabled={busy}
                onClick={async () => {
                  setBusy(true);
                  setError("");
                  try {
                    await api(`/customers/${rotating.id}/rotate-token`, {
                      method: "POST",
                      body: "{}",
                    });
                    setRotating(null);
                    await load();
                  } catch (e) {
                    setError(message(e));
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                {t("Rotate subscription")}
              </Button>
            </div>
          </div>
        </Dialog>
      )}
      {deleting && (
        <Dialog label={t("Delete")} onClose={() => !busy && setDeleting(null)}>
          <div className="space-y-4">
            <p>
              {t("Delete")} {deleting.name}?
            </p>
            {error && (
              <p role="alert" className="text-red-500">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setDeleting(null)}
              >
                {t("Cancel")}
              </Button>
              <Button
                variant="danger"
                disabled={busy}
                onClick={() => void remove()}
              >
                {t("Delete")}
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </div>
  );
}
