import { LoadingState } from "@/components/common/loading-state";
import { formatDate } from "@/lib/dates";
import { FloatingMenu } from "@/components/common/floating-menu";
import { ClientConnections } from "@/components/client-connections";
import { CustomerFreeze } from "@/components/customer-freeze";
import { Textarea } from "@/components/ui/controls";
import { Select, Input } from "@/components/ui/controls";
import { t } from "@/lib/i18n";
import { useLocale, tx } from "@/lib/i18n";
import { can, useSession } from "@/lib/session";
import { SubscriptionCheck } from "@/components/subscription-check";
import { Dialog } from "@/components/common/dialog";
import { FormEvent, useEffect, useMemo, useState, useRef } from "react";
import {
  Check,
  Copy,
  ExternalLink,
  Globe,
  MonitorSmartphone,
  Pause,
  Pencil,
  Play,
  Plus,
  RotateCcw,
  Search,
  Trash2,
  Users,
  UserRound,
  X,
} from "lucide-react";
import {
  api,
  distinctAccounts,
  effectiveMultiplier,
  type AppSettings,
  type ClientItem,
  type ClientInboundOption,
} from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/common/page-header";
import { FaIcon } from "@/components/ui/fa-icon";

const fmt = (b: number) => {
  if (!b) return t("0 B");
  const u = ["B", t("KB"), t("MB"), t("GB"), t("TB")];
  const i = Math.max(
    0,
    Math.min(Math.floor(Math.log(b) / Math.log(1024)), u.length - 1),
  );
  return `${(b / 1024 ** i).toFixed(i < 2 ? 0 : 2)} ${u[i]}`;
};
const toLocalInput = (value: string | null) =>
  value
    ? new Date(
        new Date(value).getTime() - new Date(value).getTimezoneOffset() * 60000,
      )
        .toISOString()
        .slice(0, 16)
    : "";
const expiry = (value: string | null) => {
  if (!value) return t("Never");
  const d = new Date(value),
    days = Math.ceil((d.getTime() - Date.now()) / 86400000);
  if (days < 0) return t("Expired");
  if (days === 0) return t("Today");
  if (days === 1) return t("1 day");
  return tx("{0} days", days);
};
const clientState = (client: ClientItem) =>
  client.frozen
    ? t("Frozen", "فریز شده")
    : client.enabled
      ? client.online === true
        ? t("Online")
        : client.online === false
          ? t("Offline")
          : t("Unknown")
      : client.disabled_reason === "quota"
        ? t("Quota reached")
        : client.disabled_reason === "expired"
          ? t("Expired")
          : t("Disabled");

type Editor = {
  id?: number;
  account_id?: number;
  inbound_ids?: number[];
  service_ids?: number[];
  plan_id?: number;
  count?: string;
  name: string;
  inbound: string;
  limit: string;
  days: string;
  expiry: string;
  renewal: string;
  multiplier: string;
  owner_id?: number;
  note: string;
  devices: string;
  hwidLimit: string;
  startAfter: boolean;
};
type ServiceChoice = { id: number; name: string; inbound_ids: number[] };
type ClientRow = ClientItem & { inbound_ids: number[] };
function clientRows(items: ClientItem[]): ClientRow[] {
  const groups = new Map<string, ClientRow>();
  for (const item of items) {
    const key = item.account_id
      ? `account:${item.account_id}`
      : `client:${item.id}`;
    const current = groups.get(key);
    if (!current) {
      groups.set(key, { ...item, inbound_ids: [item.inbound] });
      continue;
    }
    current.inbound_ids.push(item.inbound);
    for (const field of ["node_name", "inbound_name", "protocol"] as const)
      if (!current[field].split(" · ").includes(item[field]))
        current[field] += ` · ${item[field]}`;
    current.online =
      current.online === true || item.online === true
        ? true
        : current.online === null || item.online === null
          ? null
          : false;
    current.usage_metered = current.usage_metered && item.usage_metered;
  }
  return [...groups.values()];
}
const blank = (
  inbound = "",
  days = "30",
  limit = "0",
  renewal = "0",
): Editor => ({
  name: "",
  inbound,
  limit,
  days,
  expiry: "",
  renewal,
  multiplier: "1",
  note: "",
  devices: "0",
  hwidLimit: "0",
  startAfter: false,
});

export function ClientsPage() {
  useLocale();
  const session = useSession();
  const [inbounds, setInbounds] = useState<ClientInboundOption[]>([]),
    [services, setServices] = useState<ServiceChoice[]>([]),
    [items, setItems] = useState<ClientItem[]>([]),
    [cfg, setCfg] = useState<AppSettings | null>(null),
    [query, setQuery] = useState(""),
    [status, setStatus] = useState("all"),
    [inboundFilter, setInboundFilter] = useState("all"),
    [msg, setMsg] = useState(""),
    [copied, setCopied] = useState<number | null>(null),
    [selected, setSelected] = useState<Set<number>>(new Set()),
    [editor, setEditor] = useState<Editor | null>(null),
    [busy, setBusy] = useState(false),
    [menu, setMenu] = useState<number | null>(null);
  const [checkingClient, setCheckingClient] = useState<ClientItem | null>(null);
  const [loadingEditor, setLoadingEditor] = useState(false);
  const [loading, setLoading] = useState(true);
  const editorRequest = useRef<AbortController | null>(null);
  useEffect(() => () => editorRequest.current?.abort(), []);
  const [connections, setConnections] = useState<{
    client: ClientItem;
    kind: "ips" | "devices";
  } | null>(null);
  useEffect(() => setMenu(null), [query, status, inboundFilter]);
  const [owners, setOwners] = useState<import("@/lib/api").AdminUser[]>([]);
  useEffect(() => {
    if (session?.is_superuser)
      api<import("@/lib/api").AdminUser[]>("/admin/users/")
        .then(setOwners)
        .catch(() => {});
  }, [session]);
  const polling = useRef(false);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);
  const [liveError, setLiveError] = useState(false);
  const load = async () => {
    const [i, c, s, plans] = await Promise.all([
      api<ClientInboundOption[]>("/clients/inbound-options"),
      api<ClientItem[]>("/clients/"),
      api<AppSettings>("/settings"),
      api<ServiceChoice[]>("/services"),
    ]);
    const clientInbounds = i.filter((x) => x.supports_clients !== false);
    setInbounds(clientInbounds);
    setItems(c);
    setUpdatedAt(new Date());
    setLiveError(false);
    setCfg(s);
    setServices(plans);
    return { i: clientInbounds, c };
  };
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    load()
      .catch((e) => {
        if (active) setMsg(e.message);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    const poll = async () => {
      if (polling.current || document.hidden) return;
      polling.current = true;
      try {
        const c = await api<ClientItem[]>("/clients/", {
          signal: controller.signal,
        });
        if (active) {
          setItems(c);
          setUpdatedAt(new Date());
          setLiveError(false);
          setSelected((previous) => {
            const ids = new Set(c.map((item) => item.id));
            const next = new Set([...previous].filter((id) => ids.has(id)));
            return next.size === previous.size ? previous : next;
          });
        }
      } catch {
        if (active) setLiveError(true);
      } finally {
        polling.current = false;
      }
    };
    const interval = setInterval(poll, 5000);
    document.addEventListener("visibilitychange", poll);
    return () => {
      active = false;
      controller.abort();
      clearInterval(interval);
      document.removeEventListener("visibilitychange", poll);
    };
  }, []);
  const accounts = useMemo(() => clientRows(items), [items]);
  const filtered = useMemo(
    () =>
      accounts.filter((c) => {
        const q = query.trim().toLowerCase();
        const text =
          `${c.name} ${c.inbound_name} ${c.node_name} ${c.credential}`.toLowerCase();
        const okq = !q || text.includes(q);
        const oks =
          status === "all" ||
          (status === "active" && c.enabled) ||
          (status === "disabled" && !c.enabled) ||
          (status === "online" && c.online === true) ||
          (status === "offline" && c.online === false && c.enabled) ||
          (status === "expiring" &&
            c.enabled &&
            !!c.expires_at &&
            new Date(c.expires_at).getTime() > Date.now() &&
            new Date(c.expires_at).getTime() - Date.now() <= 7 * 86400000) ||
          (status === "near-quota" &&
            c.enabled &&
            c.traffic_limit_bytes > 0 &&
            c.used_traffic_bytes / c.traffic_limit_bytes >= 0.8);
        const oki =
          inboundFilter === "all" ||
          c.inbound_ids.includes(Number(inboundFilter));
        return okq && oks && oki;
      }),
    [accounts, query, status, inboundFilter],
  );
  const total = useMemo(
      () =>
        distinctAccounts(items).reduce((a, c) => a + c.used_traffic_bytes, 0),
      [items],
    ),
    active = accounts.filter((c) => c.enabled).length,
    limited = accounts.filter((c) => c.traffic_limit_bytes > 0).length;
  const allVisible =
    filtered.length > 0 && filtered.every((c) => selected.has(c.id));
  function toggleSelect(id: number) {
    setSelected((s) => {
      const n = new Set(s);
      n.has(id) ? n.delete(id) : n.add(id);
      return n;
    });
  }
  function toggleAll() {
    setSelected((s) => {
      const n = new Set(s);
      if (allVisible) filtered.forEach((c) => n.delete(c.id));
      else filtered.forEach((c) => n.add(c.id));
      return n;
    });
  }
  async function refresh() {
    try {
      await load();
      setMsg("");
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function copy(c: ClientItem) {
    await navigator.clipboard.writeText(c.subscription_url);
    setCopied(c.id);
    setTimeout(() => setCopied(null), 1200);
  }
  function openCreate() {
    setMsg("");
    editorRequest.current?.abort();
    setLoadingEditor(false);
    setEditor({
      ...blank(
        inbounds[0] ? String(inbounds[0].id) : "",
        String(cfg?.default_client_days ?? 30),
        String(cfg?.default_traffic_gb ?? 0),
        String(cfg?.default_renewal_days ?? 0),
      ),
      inbound_ids: inbounds[0] ? [inbounds[0].id] : [],
      service_ids: [],
    });
  }
  function openBatch() {
    setMsg("");
    editorRequest.current?.abort();
    setLoadingEditor(false);
    const first = inbounds.find((i) => i.supports_batch !== false);
    setEditor({
      ...blank(
        String(first?.id || ""),
        String(cfg?.default_client_days ?? 30),
        String(cfg?.default_traffic_gb ?? 0),
        String(cfg?.default_renewal_days ?? 0),
      ),
      count: "10",
    });
  }
  function exportUsage() {
    const fields = [
      "id",
      "name",
      "node_name",
      "inbound_name",
      "enabled",
      "used_traffic_bytes",
      "raw_used_traffic_bytes",
      "traffic_multiplier",
      "account_id",
      "traffic_limit_bytes",
      "expires_at",
      "note",
    ] as const;
    const quote = (value: unknown) => {
      let text = String(value ?? "");
      if (/^\s*[=+\-@]/.test(text)) text = "'" + text;
      return '"' + text.replaceAll('"', '""') + '"';
    };
    const lines = [
      fields.join(","),
      ...filtered.map((c) => fields.map((k) => quote((c as any)[k])).join(",")),
    ];
    const url = URL.createObjectURL(
      new Blob(["\uFEFF" + lines.join("\r\n")], {
        type: "text/csv;charset=utf-8",
      }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = "veloray-client-usage.csv";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  async function openEdit(c: ClientItem) {
    if (c.frozen) {
      setMsg(
        t(
          "Resume the subscription before editing its policy.",
          "برای ویرایش شرایط اشتراک، ابتدا آن را از فریز خارج کنید.",
        ),
      );
      return;
    }
    editorRequest.current?.abort();
    const controller = new AbortController();
    editorRequest.current = controller;
    setMsg("");
    setLoadingEditor(true);
    try {
      const shared = c.account_id
        ? await api<{
            inbound_ids: number[];
            service_ids: number[];
            plan_id?: number;
          }>(`/customers/${c.account_id}`, { signal: controller.signal })
        : null;
      if (controller.signal.aborted) return;
      setEditor({
        id: c.id,
        ...(shared
          ? {
              account_id: c.account_id,
              inbound_ids: shared.inbound_ids,
              service_ids: shared.service_ids || [],
              plan_id: shared.plan_id || 0,
            }
          : {}),
        owner_id: c.owner_id,
        name: c.name,
        inbound: String(c.inbound),
        limit: c.traffic_limit_bytes
          ? String(Number((c.traffic_limit_bytes / 1024 ** 3).toFixed(2)))
          : "0",
        days: String(c.duration_days || 30),
        devices: String(c.device_limit || 0),
        hwidLimit: String(c.hwid_limit || 0),
        startAfter: !!c.start_after_first_use,
        expiry: toLocalInput(c.expires_at),
        renewal: String(c.renewal_interval_days || 0),
        multiplier: String(c.traffic_multiplier ?? 1),
        note: (c as any).note || "",
      });
    } catch (error) {
      if (!controller.signal.aborted)
        setMsg(error instanceof Error ? error.message : String(error));
    } finally {
      if (!controller.signal.aborted) setLoadingEditor(false);
    }
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!editor) return;
    setBusy(true);
    setMsg("");
    try {
      const selectedInbound = inbounds.find(
        (i) => String(i.id) === editor.inbound,
      );
      const granted = new Set([
        ...(editor.inbound_ids || []),
        ...services
          .filter((service) => editor.service_ids?.includes(service.id))
          .flatMap((service) => service.inbound_ids),
      ]);
      const shared =
        !!editor.account_id || granted.size > 1 || !!editor.service_ids?.length;
      const usageMetered = shared
        ? inbounds
            .filter((inbound) => granted.has(inbound.id))
            .every((inbound) => inbound.usage_metered !== false)
        : !selectedInbound ||
          !["http", "socks", "tunnel", "tun"].includes(
            selectedInbound.protocol,
          );
      const multiplier = Number(editor.multiplier);
      if (!Number.isFinite(multiplier) || editor.multiplier.trim() === "")
        throw new Error(t("Enter a valid traffic multiplier."));
      const body: any = {
        name: editor.name,
        inbound: editor.inbound_ids?.[0] ?? Number(editor.inbound),
        traffic_limit_bytes: usageMetered
          ? Math.round(Math.max(0, Number(editor.limit)) * 1024 ** 3)
          : 0,
        renewal_interval_days: usageMetered
          ? Math.max(0, Number(editor.renewal) || 0)
          : 0,
        traffic_multiplier: Number(editor.multiplier),
        ...(session?.is_superuser && editor.owner_id
          ? { owner_id: editor.owner_id }
          : {}),
        note: editor.note,
        device_limit: Number(editor.devices) || 0,
        hwid_limit: Number(editor.hwidLimit) || 0,
        start_after_first_use: editor.startAfter,
        duration_days: Number(editor.days) || 30,
      };
      if (shared) {
        delete body.inbound;
        body.inbound_ids = editor.inbound_ids || [];
        body.service_ids = editor.service_ids || [];
        // Keep an existing plan without reapplying its changed defaults.
        if (!editor.plan_id) body.plan_id = 0;
      }
      if (editor.id) {
        body.expires_at =
          editor.startAfter &&
          !items.find((c) => c.id === editor.id)?.first_used_at
            ? null
            : editor.expiry
              ? new Date(editor.expiry).toISOString()
              : null;
        await api(
          editor.account_id
            ? `/customers/${editor.account_id}`
            : `/clients/${editor.id}/`,
          {
            method: "PATCH",
            body: JSON.stringify(body),
          },
        );
        setMsg(t("Client updated and Xray redeployed."));
      } else {
        const d = Math.max(0, Number(editor.days) || 0);
        body.expires_at =
          !editor.startAfter && d > 0
            ? new Date(Date.now() + d * 86400000).toISOString()
            : null;
        body.enabled = true;
        if (editor.count !== undefined) {
          delete body.enabled;
          body.count = Number(editor.count);
          await api("/clients/batch", {
            method: "POST",
            body: JSON.stringify(body),
          });
        } else
          await api(shared ? "/customers" : "/clients/", {
            method: "POST",
            body: JSON.stringify(body),
          });
      }
      setEditor(null);
      await refresh();
      setMsg(
        editor.count !== undefined
          ? tx("{0} clients created in one deployment.", editor.count)
          : editor.id
            ? t("Client updated and Xray redeployed.")
            : t("Client created and Xray redeployed."),
      );
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function toggle(c: ClientItem) {
    try {
      await api(`/clients/${c.id}/toggle/`, { method: "POST" });
      await refresh();
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function reset(c: ClientItem) {
    try {
      await api(`/clients/${c.id}/reset-usage/`, { method: "POST" });
      await refresh();
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function remove(c: ClientItem) {
    if (!confirm(tx("Delete {0}?", c.name))) return;
    try {
      await api(
        c.account_id ? `/customers/${c.account_id}` : `/clients/${c.id}/`,
        { method: "DELETE" },
      );
      await refresh();
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function rotateSubscription(c: ClientItem) {
    if (
      !c.account_id ||
      !confirm(
        t(
          "Replace this subscription link? All previous subscription links will stop working.",
          "لینک ساب عوض شود؟ همهٔ لینک‌های قبلی این اشتراک غیرفعال می‌شوند.",
        ),
      )
    )
      return;
    try {
      await api(`/customers/${c.account_id}/rotate-token`, {
        method: "POST",
        body: "{}",
      });
      await refresh();
    } catch (error) {
      setMsg(error instanceof Error ? error.message : String(error));
    }
  }
  async function bulk(action: "enable" | "disable" | "reset" | "delete") {
    const targets = accounts.filter((c) => selected.has(c.id));
    if (!targets.length) return;
    if (
      action === "delete" &&
      !confirm(tx("Delete {0} selected clients?", targets.length))
    )
      return;
    setBusy(true);
    try {
      if (targets.length > 200)
        throw new Error(t("Select up to 200 clients per operation."));
      await api("/clients/bulk", {
        method: "POST",
        body: JSON.stringify({
          ids: targets.map((c) => c.id),
          action,
          whole_accounts: true,
        }),
      });
      setSelected(new Set());
      await refresh();
      setMsg(
        tx("{0} clients updated in one deployment per node.", targets.length),
      );
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  if (loading)
    return (
      <div className="space-y-6">
        <PageHeader title={t("Clients")} />
        <LoadingState />
      </div>
    );
  if (!updatedAt && msg)
    return (
      <div className="space-y-6">
        <PageHeader title={t("Clients")} />
        <div className="workspace-error" role="alert">
          <p>{msg}</p>
          <Button
            variant="outline"
            onClick={() => {
              setLoading(true);
              setMsg("");
              void load()
                .catch((e) => setMsg(e.message))
                .finally(() => setLoading(false));
            }}
          >
            {t("Retry", "تلاش دوباره")}
          </Button>
        </div>
      </div>
    );
  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Clients")}
        description={t(
          "Manage credentials, quotas, expiry, renewals and subscriptions.",
        )}
        actions={
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              onClick={exportUsage}
              disabled={!filtered.length}
            >
              <FaIcon icon="download" /> {t("Export usage")}
            </Button>
            <Button
              variant="outline"
              onClick={openBatch}
              disabled={
                !can(session, "clients.write") ||
                !inbounds.some((i) => i.supports_batch !== false)
              }
            >
              <Users className="h-3.5 w-3.5" /> {t("Batch create")}
            </Button>
            <Button
              onClick={openCreate}
              disabled={!inbounds.length || !can(session, "clients.write")}
            >
              <Plus className="h-3.5 w-3.5" />
              {t("Add client")}
            </Button>
          </div>
        }
      />
      {msg && (
        <div
          role="status"
          aria-live="polite"
          className={`rounded-md border px-3 py-2.5 text-[12px] ${msg.toLowerCase().includes("failed") || msg.toLowerCase().includes("error") ? "border-red-500/20 bg-red-500/8 text-red-500" : "border-[var(--border)] bg-[var(--card)] text-[var(--muted-strong)]"}`}
        >
          {msg}
        </div>
      )}
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Mini
          title={t("Total clients")}
          value={String(accounts.length)}
          detail={tx("{0} active", active)}
          icon="users"
        />
        <Mini
          title={t("Recorded traffic")}
          value={fmt(total)}
          detail=""
          icon="chart-area"
        />
        <Mini
          title={t("Quota managed")}
          value={String(limited)}
          detail={tx("{0} unlimited", items.length - limited)}
          icon="gauge-high"
        />
        <Mini
          title={t("Online now")}
          value={String(items.filter((c) => c.online === true).length)}
          detail={
            liveError
              ? "Reconnecting…"
              : t("Live connections · every 5 seconds")
          }
          icon="signal"
        />
      </section>
      <Card>
        <CardHeader className="clients-directory-header">
          <div className="client-filters">
            <div className="search-field">
              <Search className="search-icon h-3.5 w-3.5 text-[var(--muted)]" />
              <Input
                className="input h-9 pl-8"
                placeholder={t("Search clients...")}
                aria-label={t("Search clients...")}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
            <Select
              className="input h-9 w-auto min-w-[120px]"
              aria-label={t("Client status", "وضعیت کلاینت")}
              value={status}
              onChange={(e) => setStatus(e.target.value)}
            >
              <option value="all">{t("All status")}</option>
              <option value="online">{t("Online")}</option>
              <option value="offline">{t("Offline")}</option>
              <option value="active">{t("Active")}</option>
              <option value="expiring">{t("Expiring within 7 days")}</option>
              <option value="near-quota">{t("Usage above 80%")}</option>
              <option value="disabled">{t("Disabled")}</option>
            </Select>
            <Select
              className="input h-9 w-auto min-w-[150px]"
              aria-label={t("Connection filter", "فیلتر اتصال")}
              value={inboundFilter}
              onChange={(e) => setInboundFilter(e.target.value)}
            >
              <option value="all">
                {t("All connections", "همهٔ اتصال‌ها")}
              </option>
              {inbounds.map((i) => (
                <option key={i.id} value={i.id}>
                  {i.name}
                </option>
              ))}
            </Select>
          </div>
        </CardHeader>
        {selected.size > 0 && (
          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border)] bg-[var(--surface-2)] px-4 py-2.5">
            <div className="text-[11px] font-medium">
              {selected.size} {t("selected")}
            </div>
            <div className="flex flex-wrap gap-1.5">
              <Button
                size="sm"
                variant="outline"
                onClick={() => bulk("enable")}
                disabled={busy}
              >
                <Play className="h-3.5 w-3.5" />
                {t("Enable")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => bulk("disable")}
                disabled={busy}
              >
                <Pause className="h-3.5 w-3.5" />
                {t("Disable")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => bulk("reset")}
                disabled={busy}
              >
                <RotateCcw className="h-3.5 w-3.5" />
                {t("Reset usage")}
              </Button>
              <Button
                size="sm"
                variant="danger"
                onClick={() => bulk("delete")}
                disabled={busy}
              >
                <Trash2 className="h-3.5 w-3.5" />
                {t("Delete")}
              </Button>
            </div>
          </div>
        )}
        <CardContent className="p-0">
          <div className="hidden md:block table-wrap">
            <table className="data-table min-w-[1040px]">
              <thead>
                <tr>
                  <th className="w-10">
                    <Input
                      type="checkbox"
                      aria-label={t("Select all visible clients")}
                      checked={allVisible}
                      onChange={toggleAll}
                    />
                  </th>
                  <th>{t("Client")}</th>
                  <th>{t("Connections", "اتصال‌ها")}</th>
                  <th>{t("Traffic")}</th>
                  <th>{t("Expiry")}</th>
                  <th>{t("Status")}</th>
                  <th>{t("Subscription")}</th>
                  <th className="w-12"></th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((c) => {
                  const p = c.traffic_limit_bytes
                    ? Math.min(
                        100,
                        (c.used_traffic_bytes / c.traffic_limit_bytes) * 100,
                      )
                    : 0;
                  return (
                    <tr key={c.id}>
                      <td>
                        <Input
                          type="checkbox"
                          aria-label={tx("Select {0}", c.name)}
                          checked={selected.has(c.id)}
                          onChange={() => toggleSelect(c.id)}
                        />
                      </td>
                      <td>
                        <div className="flex min-w-0 items-center gap-3">
                          <div
                            className={`client-avatar ${c.enabled ? "active" : ["expired", "quota"].includes(c.disabled_reason || "") ? "ended" : "disabled"}`}
                          >
                            <UserRound size={16} />
                          </div>
                          <div className="min-w-0">
                            <div className="truncate font-semibold text-[var(--fg)]">
                              {c.name}
                            </div>
                            <div className="mt-0.5 truncate text-[9px] text-[var(--muted)]">
                              {c.node_name}
                            </div>
                          </div>
                        </div>
                      </td>
                      <td>
                        <div className="font-medium text-[var(--fg)]">
                          {c.inbound_name}
                        </div>
                        <div className="mt-0.5 flex items-center gap-1 text-[9px] text-[var(--muted)]">
                          <span>{c.protocol?.toUpperCase()}</span>
                          <span>·</span>
                          <span>
                            {c.inbound_ids.length > 1
                              ? tx("{0} connections", c.inbound_ids.length)
                              : `${t("ID")} ${c.inbound}`}
                          </span>
                        </div>
                      </td>
                      <td>
                        {c.usage_metered ? (
                          <div className="min-w-[150px]">
                            <div className="flex justify-between gap-3 text-[10px]">
                              <span className="font-medium text-[var(--fg)]">
                                {fmt(c.used_traffic_bytes)} {t("Used")}{" "}
                                <span className="traffic-factor">
                                  {c.effective_traffic_multiplier ?? 1}x
                                </span>
                              </span>
                              <span>
                                {c.traffic_limit_bytes
                                  ? fmt(c.traffic_limit_bytes)
                                  : t("Unlimited")}
                              </span>
                            </div>

                            {c.traffic_limit_bytes > 0 ? (
                              <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-[var(--surface-3)]">
                                <div
                                  className={`h-full rounded-full ${p >= 90 ? "bg-red-500" : p >= 70 ? "bg-amber-500" : "bg-[var(--brand)]"}`}
                                  style={{ width: `${p}%` }}
                                />
                              </div>
                            ) : (
                              <div className="mt-2 h-1.5 rounded-full bg-[var(--surface-3)]" />
                            )}
                          </div>
                        ) : (
                          <div className="text-[10px] text-[var(--muted)]">
                            {t("Not metered per user")}
                          </div>
                        )}
                      </td>
                      <td>
                        <div className="font-medium text-[var(--fg)]">
                          {c.start_after_first_use && !c.first_used_at
                            ? `${c.duration_days} ${t("days · after first use")}`
                            : expiry(c.expires_at)}
                        </div>
                        <div className="mt-0.5 text-[9px] text-[var(--muted)]">
                          {c.expires_at
                            ? formatDate(c.expires_at)
                            : t("No expiry")}
                        </div>
                      </td>
                      <td>
                        <Badge
                          tone={
                            c.online === true
                              ? "green"
                              : c.enabled
                                ? "neutral"
                                : "amber"
                          }
                        >
                          {clientState(c)}
                        </Badge>
                      </td>
                      <td>
                        <div className="flex gap-1">
                          <Button
                            title={t("Copy subscription")}
                            variant="outline"
                            size="icon"
                            onClick={() => copy(c)}
                          >
                            {copied === c.id ? (
                              <Check className="h-3.5 w-3.5 text-emerald-500" />
                            ) : (
                              <Copy className="h-3.5 w-3.5" />
                            )}
                          </Button>
                          <Button
                            title={t("Open subscription portal")}
                            variant="ghost"
                            size="icon"
                            onClick={() =>
                              window.open(
                                c.subscription_portal_url || c.subscription_url,
                                "_blank",
                                "noopener,noreferrer",
                              )
                            }
                          >
                            <ExternalLink className="h-3.5 w-3.5" />
                          </Button>
                          {!!c.account_id && can(session, "clients.write") && (
                            <CustomerFreeze
                              id={c.account_id}
                              name={c.name}
                              frozen={c.frozen}
                              until={c.freeze_until}
                              onChanged={refresh}
                            />
                          )}
                        </div>
                      </td>
                      <td className="relative">
                        <FloatingMenu
                          label={tx("Actions for {0}", c.name)}
                          open={menu === c.id}
                          onToggle={() => setMenu(menu === c.id ? null : c.id)}
                          onClose={() => setMenu(null)}
                        >
                          <MenuItem
                            icon="pen"
                            disabled={!can(session, "clients.write")}
                            label={t("Edit")}
                            onClick={() => {
                              setMenu(null);
                              openEdit(c);
                            }}
                          />
                          <MenuItem
                            icon={c.enabled ? "pause" : "play"}
                            disabled={!can(session, "clients.write")}
                            label={c.enabled ? t("Disable") : t("Enable")}
                            onClick={() => {
                              setMenu(null);
                              toggle(c);
                            }}
                          />
                          <MenuItem
                            icon="check"
                            label={t("Check subscription")}
                            onClick={() => {
                              setMenu(null);
                              setCheckingClient(c);
                            }}
                          />
                          <MenuItem
                            icon="globe"
                            label={t("IP's")}
                            onClick={() => {
                              setMenu(null);
                              setConnections({ client: c, kind: "ips" });
                            }}
                          />
                          <MenuItem
                            icon="device"
                            label={t("Device's")}
                            onClick={() => {
                              setMenu(null);
                              setConnections({ client: c, kind: "devices" });
                            }}
                          />
                          <MenuItem
                            icon="rotate-left"
                            disabled={!can(session, "clients.write")}
                            label={t("Reset usage")}
                            onClick={() => {
                              setMenu(null);
                              reset(c);
                            }}
                          />
                          {!!c.account_id && (
                            <MenuItem
                              icon="link"
                              label={t(
                                "Replace subscription link",
                                "تغییر لینک ساب",
                              )}
                              disabled={!can(session, "clients.write")}
                              onClick={() => {
                                setMenu(null);
                                void rotateSubscription(c);
                              }}
                            />
                          )}
                          <div className="my-1 h-px bg-[var(--border)]" />
                          <MenuItem
                            icon="trash"
                            disabled={!can(session, "clients.delete")}
                            label={t("Delete")}
                            danger
                            onClick={() => {
                              setMenu(null);
                              remove(c);
                            }}
                          />
                        </FloatingMenu>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          <div className="divide-y divide-[var(--border)] md:hidden">
            {filtered.map((c) => {
              const p = c.traffic_limit_bytes
                ? Math.min(
                    100,
                    (c.used_traffic_bytes / c.traffic_limit_bytes) * 100,
                  )
                : 0;
              return (
                <div key={c.id} className="p-4">
                  <div className="flex items-start justify-between gap-3">
                    <div
                      className={`client-avatar ${c.enabled ? "active" : ["expired", "quota"].includes(c.disabled_reason || "") ? "ended" : "disabled"}`}
                    >
                      <UserRound size={16} />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-[13px] font-semibold">
                        {c.name}
                      </div>
                      <div className="mt-1 truncate text-[10px] text-[var(--muted)]">
                        {c.node_name} · {c.inbound_name}
                      </div>
                    </div>
                    <Badge
                      tone={
                        c.online === true
                          ? "green"
                          : c.enabled
                            ? "neutral"
                            : "amber"
                      }
                    >
                      {clientState(c)}
                    </Badge>
                  </div>
                  <div className="mt-4">
                    {c.usage_metered ? (
                      <>
                        <div className="flex justify-between text-[10px]">
                          <span>
                            {fmt(c.used_traffic_bytes)} {t("Used")}{" "}
                            <span className="traffic-factor">
                              {c.effective_traffic_multiplier ?? 1}x
                            </span>
                          </span>
                          <span>
                            {c.traffic_limit_bytes
                              ? fmt(c.traffic_limit_bytes)
                              : t("Unlimited")}
                          </span>
                        </div>
                        <div className="mt-2 h-1.5 rounded-full bg-[var(--surface-3)]">
                          <div
                            className="h-full rounded-full bg-[var(--brand)]"
                            style={{
                              width: `${c.traffic_limit_bytes ? p : 0}%`,
                            }}
                          />
                        </div>
                      </>
                    ) : (
                      <div className="rounded-md bg-[var(--surface-2)] px-3 py-2 text-[10px] text-[var(--muted)]">
                        {t("This protocol is not metered per managed account.")}
                      </div>
                    )}
                  </div>
                  <div className="mt-4 flex flex-wrap gap-2">
                    {!!c.account_id && can(session, "clients.write") && (
                      <>
                        <CustomerFreeze
                          id={c.account_id}
                          name={c.name}
                          frozen={c.frozen}
                          until={c.freeze_until}
                          onChanged={refresh}
                        />
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => void rotateSubscription(c)}
                        >
                          {t("Replace subscription link", "تغییر لینک ساب")}
                        </Button>
                      </>
                    )}
                    <Button size="sm" variant="outline" onClick={() => copy(c)}>
                      <Copy className="h-3.5 w-3.5" />
                      {t("Copy sub")}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={!can(session, "clients.write")}
                      onClick={() => openEdit(c)}
                    >
                      <Pencil className="h-3.5 w-3.5" />
                      {t("Edit")}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setCheckingClient(c)}
                    >
                      {t("Check subscription")}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setConnections({ client: c, kind: "ips" })}
                    >
                      <Globe size={14} />
                      {t("IP's")}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        setConnections({ client: c, kind: "devices" })
                      }
                    >
                      <MonitorSmartphone size={14} />
                      {t("Device's")}
                    </Button>
                    <Button size="sm" variant="ghost" onClick={() => toggle(c)}>
                      {c.enabled ? (
                        <Pause className="h-3.5 w-3.5" />
                      ) : (
                        <Play className="h-3.5 w-3.5" />
                      )}
                      {c.enabled ? t("Disable") : t("Enable")}
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
          {!filtered.length && (
            <div className="grid place-items-center p-12 text-center">
              <Users className="mb-3 h-6 w-6 text-[var(--muted)]" />
              <div className="text-[13px] font-medium">
                {t("No clients found")}
              </div>
              <div className="mt-1 text-[11px] text-[var(--muted)]">
                {t("Change the filters or create your first client.")}
              </div>
            </div>
          )}
        </CardContent>
      </Card>
      {checkingClient && (
        <SubscriptionCheck
          client={checkingClient}
          onClose={() => setCheckingClient(null)}
        />
      )}
      {connections && (
        <ClientConnections
          key={`${connections.client.id}-${connections.kind}`}
          client={connections.client}
          kind={connections.kind}
          onClose={() => setConnections(null)}
        />
      )}
      {editor && (
        <ClientModal
          editor={editor}
          setEditor={setEditor}
          inbounds={inbounds}
          services={services}
          owners={owners}
          onSubmit={save}
          busy={busy}
          error={msg}
        />
      )}
      {loadingEditor && <p role="status">{t("Loading…")}</p>}
    </div>
  );
}

function Mini({
  title,
  value,
  detail,
  icon,
}: {
  title: string;
  value: string;
  detail: string;
  icon: string;
}) {
  useLocale();
  return (
    <Card>
      <CardContent className="flex items-center justify-between p-4">
        <div>
          <div className="text-[10px] text-[var(--muted)]">{title}</div>
          <div className="mt-1.5 text-[21px] font-semibold tracking-[-.03em]">
            {value}
          </div>
          <div className="mt-1 text-[9px] text-[var(--muted)]">{detail}</div>
        </div>
        <div className="grid h-9 w-9 place-items-center rounded-lg bg-[var(--surface-2)] text-[var(--muted-strong)]">
          <FaIcon icon={icon} />
        </div>
      </CardContent>
    </Card>
  );
}
function MenuItem({
  icon,
  label,
  onClick,
  danger = false,
  disabled = false,
}: {
  icon: string;
  label: string;
  onClick: () => void;
  danger?: boolean;
  disabled?: boolean;
}) {
  useLocale();
  return (
    <button
      role="menuitem"
      disabled={disabled}
      onClick={onClick}
      className={`disabled:opacity-40 disabled:cursor-not-allowed flex w-full items-center gap-2 rounded px-2.5 py-2 text-left text-[11px] ${danger ? "text-red-500 hover:bg-red-500/8" : "text-[var(--muted-strong)] hover:bg-[var(--surface-2)] hover:text-[var(--fg)]"}`}
    >
      <FaIcon icon={icon} className="text-[10px]" />
      {label}
    </button>
  );
}
function ClientModal({
  editor,
  setEditor,
  inbounds,
  services,
  owners,
  onSubmit,
  busy,
  error,
}: {
  editor: Editor;
  setEditor: (e: Editor | null) => void;
  inbounds: ClientInboundOption[];
  services: ServiceChoice[];
  owners: import("@/lib/api").AdminUser[];
  onSubmit: (e: FormEvent) => void;
  busy: boolean;
  error: string;
}) {
  useLocale();
  const session = useSession();
  const set = (k: keyof Editor, v: string | number | boolean) =>
    setEditor({ ...editor, [k]: v });
  const ib = inbounds.find((i) => String(i.id) === editor.inbound);
  const grants = new Set([
    ...(editor.inbound_ids || []),
    ...services
      .filter((service) => editor.service_ids?.includes(service.id))
      .flatMap((service) => service.inbound_ids),
  ]);
  const usageMetered =
    editor.inbound_ids !== undefined
      ? inbounds
          .filter((inbound) => grants.has(inbound.id))
          .every((inbound) => inbound.usage_metered !== false)
      : !ib || !["http", "socks", "tunnel", "tun"].includes(ib.protocol);
  return (
    <Dialog
      label={
        editor.count !== undefined
          ? t("Batch create clients")
          : editor.id
            ? t("Edit client")
            : t("Add client")
      }
      onClose={() => {
        if (!busy) setEditor(null);
      }}
      padded={false}
    >
      <div className="dialog-heading flex items-start justify-between border-b border-[var(--border)] px-5 py-4">
        <div>
          <div className="text-[14px] font-semibold">
            {editor.count !== undefined
              ? t("Batch create clients")
              : editor.id
                ? t("Edit client")
                : t("Add client")}
          </div>
          <div className="mt-1 text-[10px] text-[var(--muted)]">
            {t(
              "Changes are validated and applied automatically.",
              "تغییرات خودکار بررسی و اعمال می‌شوند.",
            )}
          </div>
        </div>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t("Close")}
          disabled={busy}
          onClick={() => setEditor(null)}
        >
          <X className="h-4 w-4" />
        </Button>
      </div>
      <form onSubmit={onSubmit}>
        {error && (
          <div className="workspace-error m-5" role="alert">
            {error}
          </div>
        )}
        <div className="grid gap-4 p-5 sm:grid-cols-2">
          <label className="label sm:col-span-2">
            {editor.count !== undefined ? t("Name prefix") : t("Client name")}
            <Input
              className="input"
              value={editor.name}
              onChange={(e) => set("name", e.target.value)}
              placeholder={t("e.g. Arman - iPhone")}
              required
            />
          </label>
          {editor.count !== undefined && (
            <label className="label sm:col-span-2">
              {t("Number of clients")}
              <Input
                className="input"
                type="number"
                min="1"
                max="100"
                value={editor.count}
                onChange={(e) => set("count", e.target.value)}
                required
              />
              <span className="mt-1 text-[10px] font-normal text-[var(--muted)]">
                {t(
                  "Names use prefix-001, prefix-002… Each account gets separate credentials and a subscription address.",
                )}
              </span>
            </label>
          )}
          {editor.inbound_ids !== undefined ? (
            <fieldset className="space-y-3 sm:col-span-2">
              <legend className="label">{t("Connections", "اتصال‌ها")}</legend>
              <div className="grid max-h-48 gap-2 overflow-y-auto rounded-md border border-[var(--border)] p-3">
                {inbounds.map((inbound) => (
                  <label
                    key={inbound.id}
                    className="flex items-center gap-2 text-[12px]"
                  >
                    <Input
                      type="checkbox"
                      checked={editor.inbound_ids!.includes(inbound.id)}
                      onChange={(event) => {
                        const ids = event.target.checked
                          ? [...editor.inbound_ids!, inbound.id]
                          : editor.inbound_ids!.filter(
                              (id) => id !== inbound.id,
                            );
                        setEditor({
                          ...editor,
                          inbound_ids: ids,
                          inbound: String(ids[0] || ""),
                        });
                      }}
                    />
                    <span>
                      {inbound.name} · {inbound.node_name} ·{" "}
                      {inbound.protocol.toUpperCase()}
                    </span>
                  </label>
                ))}
              </div>
              {services.length > 0 && (
                <details>
                  <summary className="cursor-pointer text-[12px] text-[var(--muted-strong)]">
                    {t("Service groups", "گروه‌های سرویس")}
                  </summary>
                  <div className="mt-2 grid gap-2">
                    {services.map((service) => (
                      <label
                        key={service.id}
                        className="flex items-center gap-2 text-[12px]"
                      >
                        <Input
                          type="checkbox"
                          checked={
                            editor.service_ids?.includes(service.id) || false
                          }
                          onChange={(event) =>
                            setEditor({
                              ...editor,
                              service_ids: event.target.checked
                                ? [...(editor.service_ids || []), service.id]
                                : editor.service_ids?.filter(
                                    (id) => id !== service.id,
                                  ),
                              plan_id:
                                !event.target.checked &&
                                editor.plan_id === service.id
                                  ? 0
                                  : editor.plan_id,
                            })
                          }
                        />
                        {service.name}
                      </label>
                    ))}
                  </div>
                </details>
              )}
            </fieldset>
          ) : (
            <label className="label sm:col-span-2">
              {t("Inbound")}
              <Select
                className="input"
                aria-label={t("Inbound")}
                value={editor.inbound}
                onChange={(e) => set("inbound", e.target.value)}
                required
              >
                <option value="">{t("Select inbound")}</option>
                {inbounds
                  .filter(
                    (i) =>
                      editor.count === undefined || i.supports_batch !== false,
                  )
                  .map((i) => (
                    <option key={i.id} value={i.id}>
                      {i.node_name} · {i.name} · {i.protocol.toUpperCase()}
                    </option>
                  ))}
              </Select>
            </label>
          )}
          {!usageMetered && (
            <div className="sm:col-span-2 rounded-lg border border-amber-500/20 bg-amber-500/8 px-3 py-2.5 text-[10px] leading-5 text-amber-600 dark:text-amber-400">
              {t(
                "Per-client traffic is unavailable for this protocol. Use unlimited traffic and no quota renewal.",
              )}
            </div>
          )}
          <label className="label">
            {t("Traffic limit (GB)")}
            <Input
              className="input"
              type="number"
              min="0"
              step=".1"
              value={usageMetered ? editor.limit : "0"}
              onChange={(e) => set("limit", e.target.value)}
              disabled={!usageMetered}
            />
            <span className="mt-1.5 block text-[9px] font-normal text-[var(--muted)]">
              {usageMetered
                ? t("0 means unlimited.")
                : t("Per-user metering unavailable for this protocol.")}
            </span>
          </label>
          {editor.id && !editor.startAfter ? (
            <label className="label">
              {t("Expires at")}
              <Input
                className="input"
                type="datetime-local"
                value={editor.expiry}
                onChange={(e) => set("expiry", e.target.value)}
              />
            </label>
          ) : (
            <label className="label">
              {t("Expiry after (days)")}
              <Input
                className="input"
                type="number"
                min="0"
                value={editor.days}
                onChange={(e) => set("days", e.target.value)}
              />
              <span className="mt-1.5 block text-[9px] font-normal text-[var(--muted)]">
                {t("0 means no expiry.")}
              </span>
            </label>
          )}
          <label className="check-row sm:col-span-2">
            <Input
              type="checkbox"
              checked={editor.startAfter}
              onChange={(e) => set("startAfter", e.target.checked)}
            />
            <span>{t("Start after first use")}</span>
          </label>
          <label className="label">
            {t("Devices")}
            <Input
              className="input"
              type="number"
              min="0"
              max="10000"
              value={editor.devices}
              onChange={(e) => set("devices", e.target.value)}
            />
            <span className="field-hint">
              {t("0 = unlimited · concurrent IPs")}
            </span>
          </label>
          <label className="label">
            {t("HWID device limit")}
            <Input
              className="input"
              type="number"
              min={-1}
              max={1000}
              value={editor.hwidLimit}
              onChange={(e) => set("hwidLimit", e.target.value)}
            />
            <span className="field-hint">
              {t(
                "0 = panel default · -1 = disabled · requires a compatible app",
              )}
            </span>
          </label>
          <label className="label">
            {t("Quota renewal (days)")}
            <Input
              className="input"
              type="number"
              min="0"
              value={usageMetered ? editor.renewal : "0"}
              onChange={(e) => set("renewal", e.target.value)}
              disabled={!usageMetered}
            />
            <span className="mt-1.5 block text-[9px] font-normal text-[var(--muted)]">
              {usageMetered
                ? t("Reset used traffic automatically.")
                : t("Disabled without per-user metering.")}
            </span>
          </label>
          <label className="label sm:col-span-2">
            {t("Traffic multiplier")}
            <Input
              className="input"
              type="number"
              step="0.001"
              value={editor.multiplier}
              onChange={(e) => set("multiplier", e.target.value)}
              required
              disabled={!usageMetered}
              aria-describedby="multiplier-help"
              aria-label={t("Traffic multiplier")}
            />
            <span
              id="multiplier-help"
              className="mt-1 block text-[11px] font-normal leading-6 text-[var(--muted)]"
            >
              {Number.isFinite(Number(editor.multiplier))
                ? tx(
                    "1G => {0}G",
                    Math.max(0, effectiveMultiplier(Number(editor.multiplier))),
                  )
                : t("Enter a number.")}
            </span>
          </label>
          {session?.is_superuser && (
            <label className="label sm:col-span-2">
              {t("Owner")}
              <Select
                className="input"
                value={editor.owner_id || session.id}
                onChange={(e) => set("owner_id", Number(e.target.value))}
              >
                {owners.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.username} · {t(u.role)}
                  </option>
                ))}
              </Select>
            </label>
          )}
          <label className="label sm:col-span-2">
            {t("Note")}
            <Textarea
              className="input"
              value={editor.note}
              onChange={(e) => set("note", e.target.value)}
              placeholder={t("Optional internal note")}
            />
          </label>
        </div>
        <div className="dialog-actions flex justify-end gap-2 border-t border-[var(--border)] px-5 py-4">
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => setEditor(null)}
          >
            {t("Cancel")}
          </Button>
          <Button
            type="submit"
            disabled={
              busy ||
              !editor.name ||
              (editor.inbound_ids !== undefined
                ? grants.size === 0
                : !editor.inbound)
            }
          >
            {busy
              ? "Saving…"
              : editor.count !== undefined
                ? t("Create clients")
                : editor.id
                  ? t("Save changes")
                  : t("Create client")}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
