import { useEffect, useState, type FormEvent } from "react";
import { api, type NodeItem } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { formatDate } from "@/lib/dates";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/controls";
import { Dialog } from "@/components/common/dialog";
import { PageHeader } from "@/components/common/page-header";
type Result = {
  ok: boolean;
  latency_ms: number;
  status_code: number;
  checked_at?: string;
  at?: string;
  error?: string;
};
type Monitor = {
  id?: number;
  name: string;
  node: number;
  core: string;
  tag: string;
  url?: string;
  target_host?: string;
  enabled: boolean;
  interval_seconds: number;
  failure_threshold: number;
  failures?: number;
  down?: boolean;
  last_result?: Result;
};
type Billing = {
  enabled: boolean;
  provider: string;
  region: string;
  reference: string;
  currency: string;
  price_minor: number;
  budget_bytes: number;
  warning_percent: number;
  reset_day: number;
  timezone: string;
};
type Usage = {
  cycle_start?: string;
  cycle_end?: string;
  used_bytes?: number;
  lifetime_bytes?: number;
};
const initialBilling = (): Billing => ({
  enabled: true,
  provider: "",
  region: "",
  reference: "",
  currency: "USD",
  price_minor: 0,
  budget_bytes: 0,
  warning_percent: 80,
  reset_day: 1,
  timezone: "UTC",
});
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));
const gib = (n?: number) =>
  ((n || 0) / 1024 ** 3).toLocaleString(undefined, {
    maximumFractionDigits: 2,
  });
function downloadCSV(rows: (string | number | boolean | undefined)[][]) {
  const escape = (v: unknown) =>
    '"' +
    String(v ?? "")
      .replace(/^[=+@-]/, "'$&")
      .replaceAll('"', '""') +
    '"';
  const blob = new Blob(
    ["\uFEFF" + rows.map((row) => row.map(escape).join(",")).join("\r\n")],
    { type: "text/csv;charset=utf-8" },
  );
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "veloray-operations.csv";
  a.click();
  URL.revokeObjectURL(url);
}
export function OperationsPage() {
  useLocale();
  const [nodes, setNodes] = useState<NodeItem[]>([]),
    [selected, setSelected] = useState(0),
    [tab, setTab] = useState<"monitors" | "billing" | "diagnostics">(
      "monitors",
    ),
    [rows, setRows] = useState<Monitor[]>([]),
    [editor, setEditor] = useState<Monitor | null>(null),
    [deleting, setDeleting] = useState<Monitor | null>(null),
    [history, setHistory] = useState<{ name: string; rows: Result[] } | null>(
      null,
    ),
    [billing, setBilling] = useState<Billing>(initialBilling),
    [usage, setUsage] = useState<Usage>({}),
    [periods, setPeriods] = useState<Usage[]>([]),
    [diagnostics, setDiagnostics] = useState<Record<string, unknown> | null>(
      null,
    ),
    [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(""),
    [status, setStatus] = useState(""),
    [search, setSearch] = useState("");
  async function loadMonitors(signal?: AbortSignal) {
    const data = await api<Monitor[]>("/monitors", { signal });
    if (!signal?.aborted) setRows(data);
  }
  useEffect(() => {
    const c = new AbortController();
    void Promise.all([
      api<NodeItem[]>("/nodes", { signal: c.signal }),
      loadMonitors(c.signal),
    ])
      .then(([data]) => {
        if (!c.signal.aborted) {
          setNodes(data);
          setSelected(data[0]?.id || 0);
          setLoading(false);
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) {
          setError(message(e));
          setLoading(false);
        }
      });
    return () => c.abort();
  }, []);
  useEffect(() => {
    if (!selected || tab === "monitors") {
      setLoading(false);
      return;
    }
    const c = new AbortController();
    setLoading(true);
    setError("");
    setDiagnostics(null);
    setBilling(initialBilling());
    setUsage({});
    setPeriods([]);
    void (async () => {
      try {
        if (tab === "billing") {
          const data = await api<{
            config: Billing;
            usage: Usage;
            history: Usage[];
          }>(`/operations/nodes/${selected}/billing`, { signal: c.signal });
          if (!c.signal.aborted) {
            setBilling(data.config);
            setUsage(data.usage);
            setPeriods(data.history);
          }
        } else {
          const data = await api<Record<string, unknown>>(
            `/operations/nodes/${selected}/diagnostics`,
            { signal: c.signal },
          );
          if (!c.signal.aborted) setDiagnostics(data);
        }
      } catch (e) {
        if (!c.signal.aborted) setError(message(e));
      } finally {
        if (!c.signal.aborted) setLoading(false);
      }
    })();
    return () => c.abort();
  }, [selected, tab]);
  async function saveMonitor(e: FormEvent) {
    e.preventDefault();
    if (!editor || busy) return;
    setBusy(true);
    setError("");
    try {
      await api(`/monitors${editor.id ? `/${editor.id}` : ""}`, {
        method: editor.id ? "PATCH" : "POST",
        body: JSON.stringify({
          name: editor.name,
          node: editor.node,
          core: editor.core,
          tag: editor.tag,
          url: editor.url,
          enabled: editor.enabled,
          interval_seconds: editor.interval_seconds,
          failure_threshold: editor.failure_threshold,
        }),
      });
      setEditor(null);
      await loadMonitors();
      setStatus(t("Monitor saved"));
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function action(
    row: Monitor,
    kind: "probe" | "edit" | "history" | "delete",
  ) {
    setBusy(true);
    setError("");
    setStatus("");
    try {
      if (kind === "edit") setEditor(await api<Monitor>(`/monitors/${row.id}`));
      else if (kind === "history")
        setHistory({
          name: row.name,
          rows: await api<Result[]>(`/monitors/${row.id}/history`),
        });
      else if (kind === "delete") {
        await api(`/monitors/${row.id}`, { method: "DELETE" });
        setDeleting(null);
        await loadMonitors();
      } else {
        const result = await api<Result>(`/monitors/${row.id}/probe`, {
          method: "POST",
          body: "{}",
        });
        setStatus(
          t(result.ok ? "Probe succeeded" : "Probe failed") +
            ` · ${result.latency_ms} ms`,
        );
        await loadMonitors();
      }
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function saveBilling(e: FormEvent) {
    e.preventDefault();
    if (!selected || busy || loading || error) return;
    setBusy(true);
    setError("");
    try {
      await api(`/operations/nodes/${selected}/billing`, {
        method: "PATCH",
        body: JSON.stringify(billing),
      });
      const data = await api<{
        config: Billing;
        usage: Usage;
        history: Usage[];
      }>(`/operations/nodes/${selected}/billing`);
      setBilling(data.config);
      setUsage(data.usage);
      setPeriods(data.history);
      setStatus(t("Billing settings saved"));
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-5">
      <PageHeader
        title={t("Operations")}
        description={t("Outbound health, node costs and deployment history.")}
      />
      <div
        className="flex flex-wrap gap-2"
        role="tablist"
        aria-label={t("Operations")}
      >
        {(["monitors", "billing", "diagnostics"] as const).map((key) => (
          <Button
            role="tab"
            disabled={busy}
            aria-selected={tab === key}
            key={key}
            variant={tab === key ? "default" : "outline"}
            onClick={() => {
              setError("");
              setStatus("");
              setTab(key);
            }}
          >
            {t(
              key === "monitors"
                ? "Health monitors"
                : key === "billing"
                  ? "Node billing"
                  : "Diagnostics",
            )}
          </Button>
        ))}
      </div>
      <label className="label max-w-md">
        {t("Node")}
        <Select
          disabled={busy}
          value={selected}
          onChange={(e) => {
            setError("");
            setStatus("");
            setSelected(Number(e.target.value));
          }}
        >
          {nodes.map((n) => (
            <option key={n.id} value={n.id}>
              {n.name}
            </option>
          ))}
        </Select>
      </label>
      {error && !editor && !deleting && (
        <p role="alert" className="text-red-500">
          {error}
        </p>
      )}
      {status && <p role="status">{status}</p>}
      {tab === "monitors" && (
        <section className="space-y-4">
          <p className="text-[12px] text-[var(--muted)]">
            {t(
              "Probes use the selected outbound in an isolated core process. Failure alerts do not restart production traffic.",
            )}
          </p>
          <div className="flex flex-wrap gap-2">
            <Input
              aria-label={t("Search monitors")}
              placeholder={t("Search monitors")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <Button
              disabled={!selected || busy}
              onClick={() =>
                setEditor({
                  name: "",
                  node: selected,
                  core: "xray",
                  tag: "direct",
                  url: "https://www.gstatic.com/generate_204",
                  enabled: true,
                  interval_seconds: 300,
                  failure_threshold: 3,
                })
              }
            >
              {t("Add monitor")}
            </Button>
          </div>
          {rows
            .filter(
              (r) =>
                r.node === selected &&
                r.name.toLowerCase().includes(search.toLowerCase()),
            )
            .map((row) => (
              <article
                key={row.id}
                className="space-y-3 rounded-md border border-[var(--border)] p-4"
              >
                <div className="flex flex-wrap justify-between gap-2">
                  <strong className="text-[13px]">{row.name}</strong>
                  <span className="text-[12px]">
                    {t(
                      !row.enabled
                        ? "Disabled"
                        : row.down
                          ? "Down"
                          : row.last_result?.ok
                            ? "Healthy"
                            : "Pending",
                    )}{" "}
                    · {row.core} / {row.tag}
                  </span>
                </div>
                <p className="text-[11px] text-[var(--muted)]">
                  {row.target_host} · {row.last_result?.latency_ms ?? "—"} ms ·{" "}
                  {formatDate(row.last_result?.checked_at, {
                    hour: "2-digit",
                    minute: "2-digit",
                  })}
                </p>
                <div className="flex flex-wrap gap-2">
                  {(["probe", "edit", "history"] as const).map((kind) => (
                    <Button
                      key={kind}
                      size="sm"
                      variant="outline"
                      disabled={busy}
                      onClick={() => void action(row, kind)}
                    >
                      {t(
                        kind === "probe"
                          ? "Run probe"
                          : kind === "edit"
                            ? "Edit"
                            : "History",
                      )}
                    </Button>
                  ))}
                  <Button
                    size="sm"
                    variant="danger"
                    disabled={busy}
                    onClick={() => setDeleting(row)}
                  >
                    {t("Delete")}
                  </Button>
                </div>
              </article>
            ))}
          {!loading && rows.filter((r) => r.node === selected).length === 0 && (
            <p className="text-[12px]">{t("No monitors configured")}</p>
          )}
        </section>
      )}
      {tab === "billing" && selected > 0 && !loading && !error && (
        <form
          onSubmit={saveBilling}
          className="space-y-4 rounded-md border border-[var(--border)] p-4"
        >
          <h2 className="text-[14px] font-medium">{t("Node billing")}</h2>
          <label className="flex gap-2">
            <Input
              type="checkbox"
              checked={billing.enabled}
              onChange={(e) =>
                setBilling({ ...billing, enabled: e.target.checked })
              }
            />
            {t("Track node billing")}
          </label>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {(
              [
                ["provider", "Hosting provider"],
                ["region", "Region"],
                ["reference", "Account reference"],
                ["currency", "Currency"],
                ["timezone", "Billing timezone"],
              ] as const
            ).map(([key, label]) => (
              <label className="label" key={key}>
                {t(label)}
                <Input
                  value={billing[key]}
                  maxLength={key === "currency" ? 3 : 160}
                  onChange={(e) =>
                    setBilling({
                      ...billing,
                      [key]:
                        key === "currency"
                          ? e.target.value.toUpperCase()
                          : e.target.value,
                    })
                  }
                />
              </label>
            ))}
            <label className="label">
              {t("Price in minor units")}
              <Input
                type="number"
                min={0}
                max={1000000000000}
                value={billing.price_minor}
                onChange={(e) =>
                  setBilling({
                    ...billing,
                    price_minor: Number(e.target.value),
                  })
                }
              />
            </label>
            <label className="label">
              {t("Traffic budget (GiB)")}
              <Input
                type="number"
                min={0}
                max={8388607}
                step="0.01"
                value={billing.budget_bytes / 1024 ** 3}
                onChange={(e) =>
                  setBilling({
                    ...billing,
                    budget_bytes: Math.round(
                      Number(e.target.value) * 1024 ** 3,
                    ),
                  })
                }
              />
            </label>
            <label className="label">
              {t("Warning threshold (%)")}
              <Input
                type="number"
                min={1}
                max={100}
                value={billing.warning_percent}
                onChange={(e) =>
                  setBilling({
                    ...billing,
                    warning_percent: Number(e.target.value),
                  })
                }
              />
            </label>
            <label className="label">
              {t("Monthly reset day")}
              <Input
                type="number"
                min={1}
                max={31}
                value={billing.reset_day}
                onChange={(e) =>
                  setBilling({ ...billing, reset_day: Number(e.target.value) })
                }
              />
            </label>
          </div>
          <p className="text-[11px] text-[var(--muted)]">
            {t(
              "Counts tracked VPN traffic without customer multipliers. This is an estimate, not your provider invoice. Budgets send alerts without disabling a node.",
            )}
          </p>
          <p className="text-[12px]">
            {t("Current cycle")}: {gib(usage.used_bytes)} GiB ·{" "}
            {formatDate(usage.cycle_start)} → {formatDate(usage.cycle_end)}
          </p>
          <p className="text-[12px]">
            {t("Tracked lifetime")}: {gib(usage.lifetime_bytes)} GiB
          </p>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={busy}>
              {t("Save billing")}
            </Button>
            <Button
              variant="outline"
              onClick={() =>
                downloadCSV([
                  ["start", "end", "bytes"],
                  ...periods.map((p) => [
                    p.cycle_start,
                    p.cycle_end,
                    p.used_bytes,
                  ]),
                ])
              }
            >
              {t("Export billing history")}
            </Button>
          </div>
          {periods.map((p) => (
            <p className="text-[12px]" key={p.cycle_start}>
              {formatDate(p.cycle_start)} → {formatDate(p.cycle_end)} ·{" "}
              {gib(p.used_bytes)} GiB
            </p>
          ))}
        </form>
      )}
      {tab === "diagnostics" && (
        <section className="rounded-md border border-[var(--border)] p-4">
          <h2 className="text-[14px] font-medium">
            {t("Deployment diagnostics")}
          </h2>
          <p className="my-3 text-[11px] text-[var(--muted)]">
            {t(
              "Shows runtime status and recent operation IDs, states and hashes. Credentials and full configurations are excluded.",
            )}
          </p>
          {diagnostics && (
            <pre
              dir="ltr"
              className="max-h-[65vh] overflow-auto whitespace-pre-wrap break-all text-[11px]"
            >
              {JSON.stringify(diagnostics, null, 2)}
            </pre>
          )}
        </section>
      )}
      {loading && tab !== "monitors" && <p role="status">{t("Loading…")}</p>}
      {editor && (
        <Dialog
          label={t("Health monitor")}
          onClose={() => !busy && setEditor(null)}
        >
          <form onSubmit={saveMonitor} className="space-y-4">
            <h2>{t("Health monitor")}</h2>
            <label className="label">
              {t("Name")}
              <Input
                required
                maxLength={96}
                value={editor.name}
                onChange={(e) => setEditor({ ...editor, name: e.target.value })}
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="label">
                {t("Core")}
                <Select
                  value={editor.core}
                  onChange={(e) =>
                    setEditor({ ...editor, core: e.target.value })
                  }
                >
                  <option value="xray">Xray</option>
                  <option value="singbox">sing-box</option>
                </Select>
              </label>
              <label className="label">
                {t("Outbound tag")}
                <Input
                  required
                  value={editor.tag}
                  maxLength={96}
                  onChange={(e) =>
                    setEditor({ ...editor, tag: e.target.value })
                  }
                />
              </label>
            </div>
            <label className="label">
              {t("HTTPS probe URL")}
              <Input
                type="url"
                required
                value={editor.url || ""}
                onChange={(e) => setEditor({ ...editor, url: e.target.value })}
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="label">
                {t("Check interval (seconds)")}
                <Input
                  type="number"
                  min={60}
                  max={86400}
                  required
                  value={editor.interval_seconds}
                  onChange={(e) =>
                    setEditor({
                      ...editor,
                      interval_seconds: Number(e.target.value),
                    })
                  }
                />
              </label>
              <label className="label">
                {t("Failures before alert")}
                <Input
                  type="number"
                  min={1}
                  max={10}
                  required
                  value={editor.failure_threshold}
                  onChange={(e) =>
                    setEditor({
                      ...editor,
                      failure_threshold: Number(e.target.value),
                    })
                  }
                />
              </label>
            </div>
            <label className="flex gap-2">
              <Input
                type="checkbox"
                checked={editor.enabled}
                onChange={(e) =>
                  setEditor({ ...editor, enabled: e.target.checked })
                }
              />
              {t("Enabled")}
            </label>
            {error && (
              <p role="alert" className="text-red-500">
                {error}
              </p>
            )}
            <Button type="submit" disabled={busy}>
              {t("Save monitor")}
            </Button>
          </form>
        </Dialog>
      )}
      {deleting && (
        <Dialog
          label={t("Delete monitor")}
          onClose={() => !busy && setDeleting(null)}
        >
          <p>{deleting.name}</p>
          {error && <p role="alert">{error}</p>}
          <Button
            variant="danger"
            disabled={busy}
            onClick={() => void action(deleting, "delete")}
          >
            {t("Confirm delete")}
          </Button>
        </Dialog>
      )}
      {history && (
        <Dialog label={t("Probe history")} onClose={() => setHistory(null)}>
          <h2>{history.name}</h2>
          <Button
            variant="outline"
            onClick={() =>
              downloadCSV([
                ["at", "healthy", "latency_ms", "http_status"],
                ...history.rows.map((r) => [
                  r.at,
                  r.ok,
                  r.latency_ms,
                  r.status_code,
                ]),
              ])
            }
          >
            {t("Export history")}
          </Button>
          <div className="my-3 space-y-2">
            {history.rows.map((r, i) => (
              <p className="text-[12px]" key={r.at || i}>
                {formatDate(r.at, { hour: "2-digit", minute: "2-digit" })} ·{" "}
                {t(r.ok ? "Healthy" : "Failed")} · {r.latency_ms} ms · HTTP{" "}
                {r.status_code}
              </p>
            ))}
          </div>
        </Dialog>
      )}
    </div>
  );
}
