import { canVisit } from "@/lib/navigation";
import {
  DashboardSparkline,
  ResourceCard,
  type ResourceSample,
} from "@/components/dashboard-resources";
import { ChartRange } from "@/components/chart-range";
import { t } from "@/lib/i18n";
import { useLocale, tx } from "@/lib/i18n";
import { can, isReseller, useSession } from "@/lib/session";
import * as echarts from "echarts/core";
import { LineChart } from "echarts/charts";
import {
  GridComponent,
  LegendComponent,
  TooltipComponent,
  DataZoomComponent,
} from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
echarts.use([
  LineChart,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  DataZoomComponent,
  CanvasRenderer,
]);
import { useTheme } from "@/components/theme/theme-provider";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowUpRight,
  ArrowDownLeft,
  RefreshCw,
  Server,
  WifiOff,
  Eye,
  EyeOff,
} from "lucide-react";
import {
  api,
  API_BASE,
  type AppSettings,
  type NodeItem,
  type SystemInfo,
} from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { FaIcon } from "@/components/ui/fa-icon";

type Overview = {
  max_clients?: number;
  allocated_traffic_bytes?: number;
  traffic_budget_bytes?: number;
  reserved_clients?: number;
  reserved_traffic_bytes?: number;
  nodes_total: number;
  nodes_online: number;
  clients_total: number;
  clients_active: number;
  clients_expiring_7d: number;
  traffic_raw_bytes: number;
  traffic_upload_bytes: number;
  traffic_download_bytes: number;
  traffic_limit_bytes: number;
  inbounds_total: number;
  protocols: { protocol: string; count: number }[];
};
type TrafficPoint = {
  at: string;
  used_traffic_bytes?: number;
  bytes?: number;
  raw_upload_traffic_bytes: number;
  raw_download_traffic_bytes: number;
  upload_bytes: number;
  download_bytes: number;
  interval_seconds: number;
  active_clients: number;
  online_nodes: number;
};
const fmt = (b: number) => {
  if (!b) return t("0 B");
  const u = ["B", t("KB"), t("MB"), t("GB"), t("TB")];
  const i = Math.max(
    0,
    Math.min(Math.floor(Math.log(b) / Math.log(1024)), u.length - 1),
  );
  return `${(b / 1024 ** i).toFixed(i < 2 ? 0 : 2)} ${u[i]}`;
};
const uptime = (seconds: number | undefined) => {
  if (typeof seconds !== "number" || !Number.isFinite(seconds)) return "—";
  const minutes = Math.floor(Math.max(0, seconds) / 60),
    hours = Math.floor(minutes / 60),
    days = Math.floor(hours / 24);
  return days
    ? `${days}${t("d", "ر")} ${hours % 24}${t("h", "س")}`
    : hours
      ? `${hours}${t("h", "س")} ${minutes % 60}${t("m", "د")}`
      : `${minutes}${t("m", "د")}`;
};
const names: Record<string, string> = {
  vless: t("VLESS"),
  vmess: t("VMess"),
  trojan: t("Trojan"),
  shadowsocks: t("Shadowsocks"),
  hysteria: "Hysteria2",
  wireguard: t("WireGuard"),
  http: t("HTTP"),
  socks: "SOCKS5",
};
function Metric({
  label,
  value,
  note,
  icon,
  tone = "neutral",
}: {
  label: string;
  value: string;
  note: string;
  icon: string;
  tone?: "neutral" | "green" | "blue" | "amber";
}) {
  useLocale();
  const tones = {
    neutral: "text-[var(--muted-strong)] bg-[var(--surface-2)]",
    green: "text-emerald-500 bg-emerald-500/10",
    blue: "text-sky-500 bg-sky-500/10",
    amber: "text-amber-500 bg-amber-500/10",
  };
  return (
    <Card>
      <CardContent className="p-4 sm:p-5">
        <div className="flex items-center justify-between gap-2">
          <div className="text-[11px] font-medium text-[var(--muted)]">
            {label}
          </div>
          <div
            className={`grid h-7 w-7 shrink-0 place-items-center rounded-lg ${tones[tone]}`}
          >
            <FaIcon icon={icon} />
          </div>
        </div>
        <div className="metric-value mt-2 text-[24px] font-semibold tracking-tight text-[var(--fg)]">
          {value}
        </div>
        <div className="mt-1 min-h-8 text-[11px] leading-4 text-[var(--muted)]">
          {note}
        </div>
      </CardContent>
    </Card>
  );
}
export function TrafficChart({
  points,
  loading,
  error,
  onRetry,
}: {
  points: TrafficPoint[];
  loading: boolean;
  error: string;
  onRetry: () => void;
}) {
  const locale = useLocale();
  const root = useRef<HTMLDivElement>(null),
    chart = useRef<echarts.ECharts | null>(null);
  const { resolved } = useTheme();
  const [mode, setMode] = useState("usage");
  const [windowRange, setWindowRange] = useState<[number, number]>([0, 100]);
  const range = useRef<[number, number]>([0, 100]);
  const suppressZoomEvent = useRef(false);
  const preview = useMemo(
    () =>
      points.map((point) => ({
        at: point.at,
        bytes: Math.max(
          Number(point.used_traffic_bytes || 0),
          Number(point.raw_upload_traffic_bytes || 0) +
            Number(point.raw_download_traffic_bytes || 0),
        ),
      })),
    [points],
  );
  const updateRange = (next: [number, number]) => {
    range.current = next;
    setWindowRange(next);
    suppressZoomEvent.current = true;
    try {
      chart.current?.dispatchAction({
        type: "dataZoom",
        start: next[0],
        end: next[1],
      });
    } finally {
      suppressZoomEvent.current = false;
    }
  };
  useEffect(() => {
    if (!root.current) return;
    chart.current = echarts.init(root.current);
    chart.current.on("datazoom", (raw: unknown) => {
      if (suppressZoomEvent.current || !raw || typeof raw !== "object") return;
      const event = raw as {
        start?: number;
        end?: number;
        batch?: { start?: number; end?: number }[];
      };
      const data = event.batch?.[0] || event;
      if (typeof data.start !== "number" || typeof data.end !== "number")
        return;
      const next: [number, number] = [data.start, data.end];
      range.current = next;
      setWindowRange((old) =>
        old[0] === next[0] && old[1] === next[1] ? old : next,
      );
    });
    const observer = new ResizeObserver(() => chart.current?.resize());
    observer.observe(root.current);
    return () => {
      observer.disconnect();
      chart.current?.dispose();
      chart.current = null;
    };
  }, []);
  useEffect(() => {
    const css = getComputedStyle(document.documentElement),
      color = css.getPropertyValue("--chart-upload").trim();
    const downloadColor = css.getPropertyValue("--chart-download").trim();
    const directions = [
      {
        name: t("Upload"),
        color,
        cumulative: "raw_upload_traffic_bytes",
        interval: "upload_bytes",
      },
      {
        name: t("Download"),
        color: downloadColor,
        cumulative: "raw_download_traffic_bytes",
        interval: "download_bytes",
      },
    ];
    // Old accounting records can have totals without directional counters.
    // Keep that measured traffic visible instead of plotting two zero lines.
    if (
      points.some(
        (p) =>
          (p.used_traffic_bytes || 0) >
          (p.raw_upload_traffic_bytes || 0) +
            (p.raw_download_traffic_bytes || 0),
      )
    ) {
      directions.push({
        name: t("Earlier traffic"),
        color: css.getPropertyValue("--muted").trim(),
        cumulative: "earlier",
        interval: "earlier",
      });
    }
    const series = directions.map((direction) => ({
      name: direction.name,
      type: "line",
      smooth: 0.35,
      smoothMonotone: "x",
      symbol: "circle",
      symbolSize: 5,
      emphasis: { focus: "series", scale: 1.4 },
      showSymbol: points.length < 2,
      sampling: "lttb",
      lineStyle: {
        color: direction.color,
        width: 1.8,
        cap: "round",
        join: "round",
      },
      itemStyle: { color: direction.color },
      areaStyle: {
        color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
          { offset: 0, color: direction.color + "18" },
          { offset: 1, color: direction.color + "00" },
        ]),
      },
      data: points.map((p) => [
        new Date(p.at).getTime(),
        direction.cumulative === "earlier"
          ? Math.max(
              0,
              Number((mode === "usage" ? p.used_traffic_bytes : p.bytes) || 0) -
                Number(
                  mode === "usage"
                    ? p.raw_upload_traffic_bytes || 0
                    : p.upload_bytes || 0,
                ) -
                Number(
                  mode === "usage"
                    ? p.raw_download_traffic_bytes || 0
                    : p.download_bytes || 0,
                ),
            ) / (mode === "usage" ? 1 : Math.max(1, p.interval_seconds || 300))
          : mode === "usage"
            ? Number(p[direction.cumulative as keyof TrafficPoint] || 0)
            : Number(p[direction.interval as keyof TrafficPoint] || 0) /
              Math.max(1, p.interval_seconds || 300),
      ]),
    }));
    chart.current?.setOption(
      {
        animation: !matchMedia("(prefers-reduced-motion: reduce)").matches,
        animationDuration: 900,
        animationDurationUpdate: 450,
        animationEasing: "cubicOut",
        animationEasingUpdate: "cubicOut",
        textStyle: { fontFamily: "Inter, Vazirmatn, sans-serif" },
        grid: { left: 8, right: 12, top: 48, bottom: 10, containLabel: true },
        legend: {
          top: 4,
          left: 8,
          itemWidth: 16,
          itemHeight: 3,
          itemGap: 22,
          textStyle: {
            color: css.getPropertyValue("--muted-strong").trim(),
            fontSize: 11,
          },
          inactiveColor: css.getPropertyValue("--muted").trim(),
          icon: "roundRect",
        },
        tooltip: {
          className: "chart-tooltip",
          trigger: "axis",
          confine: true,
          backgroundColor: css.getPropertyValue("--card"),
          borderColor: css.getPropertyValue("--border"),
          textStyle: { color: css.getPropertyValue("--fg") },
          padding: [10, 14],
          extraCssText:
            "border-radius:8px;box-shadow:0 8px 28px #0002;font-size:12px;",
          axisPointer: {
            type: "line",
            lineStyle: {
              color: css.getPropertyValue("--muted"),
              type: "dashed",
            },
          },
          valueFormatter: (v: unknown) =>
            fmt(Number(v)) + (mode === "rate" ? "/s" : ""),
        },
        xAxis: {
          type: "time",
          boundaryGap: false,
          splitNumber: 6,
          axisLine: { show: false },
          axisTick: { show: false },
          axisLabel: {
            color: css.getPropertyValue("--muted"),
            hideOverlap: true,
            fontSize: 10,
          },
          splitLine: { show: false },
        },
        yAxis: {
          type: "value",
          min: 0,
          splitNumber: 3,
          axisLine: { show: false },
          axisTick: { show: false },
          axisLabel: {
            color: css.getPropertyValue("--muted"),
            fontSize: 10,
            formatter: (v: number) => fmt(v) + (mode === "rate" ? "/s" : ""),
          },
          splitLine: {
            lineStyle: {
              color: css.getPropertyValue("--chart-grid").trim(),
              type: [3, 5],
              width: 1,
              opacity: 0.6,
            },
          },
        },
        dataZoom: [
          {
            type: "inside",
            zoomOnMouseWheel: false,
            start: range.current[0],
            end: range.current[1],
          },
        ],
        series,
      },
      true,
    );
  }, [points, resolved, mode, locale]);
  return (
    <div className="traffic-chart-shell">
      <div className="chart-mode">
        <button
          type="button"
          className={mode === "usage" ? "selected" : ""}
          onClick={() => setMode("usage")}
        >
          {t("24h usage")}
        </button>
        <button
          type="button"
          className={mode === "rate" ? "selected" : ""}
          onClick={() => setMode("rate")}
        >
          {t("Transfer rate")}
        </button>
      </div>
      <div
        ref={root}
        className="traffic-chart-canvas h-[300px] w-full"
        role="img"
        aria-label={t("Traffic chart")}
      />
      <ChartRange points={preview} value={windowRange} onChange={updateRange} />
      {loading && !points.length && (
        <p role="status" className="text-center text-xs text-[var(--muted)]">
          {t("Loading…")}
        </p>
      )}
      {error && (
        <div
          role="status"
          className="flex flex-wrap items-center justify-center gap-3 text-xs text-red-500"
        >
          <span>{error}</span>
          <Button variant="outline" onClick={onRetry} disabled={loading}>
            <RefreshCw className="h-3.5 w-3.5" />
            {t("Refresh")}
          </Button>
        </div>
      )}
      {!loading && !error && !points.length && (
        <p className="text-center text-xs text-[var(--muted)]">
          {t("No traffic recorded yet.")}
        </p>
      )}
    </div>
  );
}
export function Dashboard() {
  useLocale();
  const session = useSession();
  const reseller = isReseller(session);
  const [o, setO] = useState<Overview | null>(null),
    [nodes, setNodes] = useState<NodeItem[]>([]),
    [history, setHistory] = useState<TrafficPoint[]>([]),
    [cfg, setCfg] = useState<AppSettings | null>(null),
    [sys, setSys] = useState<SystemInfo | null>(null),
    [msg, setMsg] = useState(""),
    [historyError, setHistoryError] = useState(""),
    [busy, setBusy] = useState(false);
  const [resourceHistory, setResourceHistory] = useState<ResourceSample[]>([]);
  const [showAddress, setShowAddress] = useState(false);
  useEffect(() => {
    if (!session || typeof EventSource === "undefined") return;
    const stream = new EventSource(`${API_BASE}/events`, {
      withCredentials: true,
    });
    const update = (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data);
        if (data.overview && typeof data.overview === "object")
          setO(data.overview);
      } catch {
        /* Polling remains available. */
      }
    };
    stream.addEventListener("overview", update as EventListener);
    stream.addEventListener("authorization-expired", () => stream.close());
    return () => stream.close();
  }, [session?.id]);
  const inFlight = useRef(false),
    request = useRef<AbortController | null>(null);
  const load = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    const controller = new AbortController();
    request.current = controller;
    const options = { signal: controller.signal };
    setBusy(true);
    try {
      const [ov, n, h, s, system] = await Promise.allSettled([
        api<Overview>("/overview", options),
        can(session, "nodes.read")
          ? api<NodeItem[]>("/nodes/", options)
          : Promise.resolve([]),
        api<TrafficPoint[]>("/traffic-history?hours=24", options),
        api<AppSettings>("/settings", options),
        can(session, "nodes.read")
          ? api<SystemInfo>("/system-info", options)
          : Promise.resolve(null),
      ]);
      if (controller.signal.aborted) return;
      // A server-info or client-list failure must not discard valid history.
      if (ov.status === "fulfilled") setO(ov.value);
      if (n.status === "fulfilled") setNodes(n.value);
      if (h.status === "fulfilled") {
        setHistory(
          (Array.isArray(h.value) ? h.value : []).filter((point) =>
            Number.isFinite(Date.parse(point.at)),
          ),
        );
        setHistoryError("");
      } else
        setHistoryError(
          h.reason instanceof Error
            ? h.reason.message
            : t("Traffic history could not load."),
        );
      if (s.status === "fulfilled") setCfg(s.value);
      if (system.status === "fulfilled") {
        setSys(system.value);
        if (system.value) {
          const info = system.value;
          const measured = (value: number) =>
            Number.isFinite(value) ? Math.max(0, Math.min(100, value)) : null;
          setResourceHistory((previous) => [
            ...previous.slice(-239),
            {
              at: Date.now(),
              cpu: measured(info.cpu_percent),
              memory: measured(info.memory_percent),
              swap: measured(info.swap_percent),
              disk: measured(info.disk_percent),
            },
          ]);
        }
      }
      const failure = [ov, n, s, system].find(
        (result) => result.status === "rejected",
      );
      setMsg(
        failure?.status === "rejected"
          ? String(failure.reason?.message || failure.reason)
          : "",
      );
    } catch (e: any) {
      if (!controller.signal.aborted) setMsg(e.message);
    } finally {
      if (request.current === controller) {
        inFlight.current = false;
        if (!controller.signal.aborted) setBusy(false);
      }
    }
  }, [session]);
  useEffect(() => {
    setResourceHistory([]);
    setShowAddress(false);
    void load();
    return () => {
      request.current?.abort();
      inFlight.current = false;
    };
  }, [load]);
  useEffect(() => {
    const id = setInterval(
      load,
      Math.max(5, cfg?.dashboard_refresh_seconds || 15) * 1000,
    );
    return () => clearInterval(id);
  }, [cfg?.dashboard_refresh_seconds, load]);
  const local = nodes.find((n) => n.is_local),
    disabled = Math.max(0, (o?.clients_total ?? 0) - (o?.clients_active ?? 0)),
    maxProto = Math.max(1, ...(o?.protocols || []).map((x) => x.count));
  return (
    <div className="dashboard-page space-y-4">
      <div className="dashboard-toolbar">
        <div className="dashboard-toolbar-title">
          <h2>{t("Dashboard")}</h2>
          {!reseller && (
            <span
              className={`dashboard-node-status ${local?.status === "ok" ? "node-online" : ""}`}
            >
              <i />
              {local
                ? `${local.name} · ${local.status === "ok" ? t("online") : local.status || t("Unknown")}`
                : t("No nodes registered")}
            </span>
          )}
        </div>
        <div className="dashboard-actions">
          {!reseller && (
            <>
              {canVisit(session, "xray") && (
                <a href="/xray">
                  <FaIcon icon="microchip" />
                  {t("Cores")}
                </a>
              )}
              {canVisit(session, "operations") && (
                <a href="/operations">
                  <FaIcon icon="chart-line" />
                  {t("Operations")}
                </a>
              )}
              {canVisit(session, "settings") && (
                <a href="/settings">
                  <FaIcon icon="gear" />
                  {t("Settings")}
                </a>
              )}
            </>
          )}
          <Button variant="outline" onClick={load} disabled={busy}>
            <RefreshCw
              className={`h-3.5 w-3.5 ${busy ? "animate-spin" : ""}`}
            />
            {t("Refresh")}
          </Button>
        </div>
      </div>
      {cfg?.announcement && (
        <div className="rounded-lg border border-[var(--brand)] bg-[var(--surface-2)] px-4 py-3 text-[11px] text-[var(--muted-strong)]">
          <FaIcon icon="bullhorn" className="mr-2 text-[var(--brand)]" />
          {cfg.announcement}
        </div>
      )}
      {msg && (
        <div
          role="status"
          aria-live="polite"
          className="rounded-md border border-red-500/20 bg-red-500/8 px-3 py-2.5 text-[12px] text-red-500"
        >
          {msg}
        </div>
      )}
      {nodes.some((n) => n.enabled && n.status !== "ok") && (
        <div
          role="status"
          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-500/25 bg-amber-500/5 px-4 py-3"
        >
          <div className="text-[12px]">
            <div className="font-semibold text-amber-500">
              {t("Nodes need attention")}
            </div>
            <p className="mt-1 text-[var(--muted-strong)]">
              {nodes
                .filter((n) => n.enabled && n.status !== "ok")
                .map((n) => `${n.name}: ${n.status || "pending"}`)
                .join(" · ")}
            </p>
          </div>
          <a
            href="/xray"
            className="text-[12px] font-semibold text-[var(--fg)] underline underline-offset-4"
          >
            {t("Inspect Xray")}
          </a>
        </div>
      )}
      {reseller && o?.clients_total === 0 && (
        <Card>
          <CardContent className="flex flex-wrap items-center justify-between gap-3 p-5">
            <p className="text-[13px] text-[var(--muted-strong)]">
              {t(
                "Create your first user from your assigned connections.",
                "اولین کاربر را با اتصال‌های واگذارشده به حسابت بساز.",
              )}
            </p>
            <a
              href="/clients"
              className="text-[13px] font-medium text-[var(--brand)]"
            >
              {t("Add client")}
            </a>
          </CardContent>
        </Card>
      )}
      {!reseller && o && (o.inbounds_total === 0 || o.clients_total === 0) && (
        <Card>
          <CardHeader>
            <CardTitle>{t("Set up your first connection")}</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-3 sm:grid-cols-3">
            {[
              {
                href: "/inbounds",
                title: t("1. Create an inbound"),
                text: t("Choose a port, protocol and connection security."),
                done: o.inbounds_total > 0,
              },
              {
                href: "/clients",
                title: t("2. Add a client"),
                text: t("Set credentials, traffic quota and expiry."),
                done: o.clients_total > 0,
              },
              {
                href: "/clients",
                title: t("3. Share the subscription"),
                text: t("Open the client portal or copy a connection link."),
                done: false,
              },
            ].map((step) => (
              <a
                key={step.title}
                href={step.href}
                className="rounded-lg border border-[var(--border)] bg-[var(--surface-2)] p-4 transition hover:border-[var(--brand)]"
              >
                <div className="text-[12px] font-semibold">
                  {step.title}
                  {step.done ? " ✓" : ""}
                </div>
                <p className="mt-2 text-[11px] leading-5 text-[var(--muted)]">
                  {step.text}
                </p>
              </a>
            ))}
          </CardContent>
        </Card>
      )}
      {reseller ? (
        <section className="grid grid-cols-2 gap-3 xl:grid-cols-4">
          <Metric
            label={t("Total clients")}
            value={String(o?.clients_total ?? 0)}
            note={
              o?.max_clients
                ? tx(
                    "{0} of {1} available user slots",
                    Math.max(
                      0,
                      o.max_clients -
                        (o.clients_total || 0) -
                        (o.reserved_clients || 0),
                    ),
                    o.max_clients,
                  )
                : t("No account count limit", "بدون سقف تعداد کاربر")
            }
            icon="users"
            tone="green"
          />
          <Metric
            label={t("Active clients")}
            value={String(o?.clients_active ?? 0)}
            note={tx("{0} disabled", disabled)}
            icon="user-check"
            tone="green"
          />
          <Metric
            label={t("Allocated traffic", "حجم تخصیص‌یافته")}
            value={
              (o?.allocated_traffic_bytes ?? 0) < 0
                ? t("Unlimited")
                : fmt(o?.allocated_traffic_bytes ?? 0)
            }
            note={
              o?.traffic_budget_bytes
                ? tx(
                    "{0} remaining of {1}",
                    fmt(
                      Math.max(
                        0,
                        o.traffic_budget_bytes -
                          (o.allocated_traffic_bytes || 0) -
                          (o.reserved_traffic_bytes || 0),
                      ),
                    ),
                    fmt(o.traffic_budget_bytes),
                  )
                : t("No traffic allocation limit", "بدون سقف تخصیص حجم")
            }
            icon="chart-area"
          />
          <Metric
            label={t("Expiring soon")}
            value={String(o?.clients_expiring_7d ?? 0)}
            note={t("Within the next 7 days")}
            icon="hourglass-half"
            tone="amber"
          />
        </section>
      ) : (
        <section
          className="dashboard-resource-grid"
          aria-label={t("Server resources", "منابع سرور")}
        >
          <ResourceCard
            label={t("CPU")}
            icon="microchip"
            value={sys?.cpu_percent}
            note={
              sys
                ? `${sys.cpu_count} ${t(sys.cpu_count === 1 ? "Core" : "Cores", "هسته")}`
                : "—"
            }
            samples={resourceHistory.map((p) => p.cpu)}
          />
          <ResourceCard
            label={t("Memory")}
            icon="memory"
            value={sys?.memory_percent}
            note={
              sys
                ? `${fmt(sys.memory_used_bytes)} / ${fmt(sys.memory_total_bytes)}`
                : "—"
            }
            samples={resourceHistory.map((p) => p.memory)}
          />
          <ResourceCard
            label={t("Swap")}
            icon="arrows-rotate"
            value={sys?.swap_percent}
            note={
              sys
                ? sys.swap_total_bytes
                  ? `${fmt(sys.swap_used_bytes)} / ${fmt(sys.swap_total_bytes)}`
                  : t("Not configured")
                : "—"
            }
            samples={resourceHistory.map((p) => p.swap)}
          />
          <ResourceCard
            label={t("Disk")}
            icon="hard-drive"
            value={sys?.disk_percent}
            note={
              sys
                ? `${fmt(sys.disk_used_bytes)} / ${fmt(sys.disk_total_bytes)}`
                : "—"
            }
            samples={resourceHistory.map((p) => p.disk)}
          />
        </section>
      )}
      <section
        className={`dashboard-main-grid ${reseller ? "dashboard-reseller-grid" : ""}`}
      >
        <Card className="dashboard-traffic-card">
          <CardHeader>
            <div>
              <CardTitle>{t("Traffic overview")}</CardTitle>
            </div>
            <div className="text-right">
              <div className="text-[18px] font-semibold">
                {fmt(o?.traffic_raw_bytes ?? 0)}
              </div>
              <div className="text-[9px] text-[var(--muted)]">
                {t("recorded total")}
              </div>
            </div>
          </CardHeader>
          <CardContent className="p-3 sm:p-5">
            {o &&
              o.traffic_raw_bytes >
                (o.traffic_upload_bytes || 0) +
                  (o.traffic_download_bytes || 0) && (
                <div className="text-[11px] text-[var(--muted)] mb-3">
                  {t("Earlier traffic")}:{" "}
                  {fmt(
                    o.traffic_raw_bytes -
                      (o.traffic_upload_bytes || 0) -
                      (o.traffic_download_bytes || 0),
                  )}
                </div>
              )}
            <TrafficChart
              points={history}
              loading={busy}
              error={historyError}
              onRetry={load}
            />
            <div className="traffic-directions">
              <div>
                <span className="traffic-direction-label">
                  <ArrowUpRight size={15} />
                  {t("Upload")}
                </span>
                <strong>{fmt(o?.traffic_upload_bytes ?? 0)}</strong>
              </div>
              <div>
                <span className="traffic-direction-label download">
                  <ArrowDownLeft size={15} />
                  {t("Download")}
                </span>
                <strong>{fmt(o?.traffic_download_bytes ?? 0)}</strong>
              </div>
            </div>
          </CardContent>
        </Card>
        {!reseller && (
          <Card className="dashboard-client-card">
            <CardHeader>
              <CardTitle>{t("Active clients")}</CardTitle>
              <span className="dashboard-side-caption">
                {t("Last 24 hours", "۲۴ ساعت گذشته")}
              </span>
            </CardHeader>
            <CardContent>
              <div className="client-primary-value">
                {o ? o.clients_active : "—"}
                <span>{t("active", "فعال")}</span>
              </div>
              <p className="client-total-note">
                {tx(
                  "{0} total · {1} disabled",
                  o?.clients_total ?? 0,
                  disabled,
                )}
              </p>
              <DashboardSparkline
                values={history
                  .map((p) => p.active_clients)
                  .filter(
                    (value) =>
                      typeof value === "number" && Number.isFinite(value),
                  )}
                label={t("Active client history", "سابقهٔ کاربران فعال")}
                className="client-history-chart"
              />
              <div className="dashboard-client-stats">
                <div>
                  <span>{t("Healthy nodes")}</span>
                  <strong>
                    {o ? `${o.nodes_online}/${o.nodes_total}` : "—"}
                  </strong>
                </div>
                <div>
                  <span>{t("Inbounds")}</span>
                  <strong>{o?.inbounds_total ?? "—"}</strong>
                </div>
                <div>
                  <span>{t("Expiring soon")}</span>
                  <strong>{o?.clients_expiring_7d ?? "—"}</strong>
                </div>
              </div>
              <div className="dashboard-protocols">
                <h4>{t("Protocol mix")}</h4>
                {o?.protocols?.length ? (
                  o.protocols.map((p) => (
                    <div className="dashboard-protocol" key={p.protocol}>
                      <span>{names[p.protocol] || p.protocol}</span>
                      <div>
                        <i
                          style={{
                            width: `${Math.max(5, (p.count / maxProto) * 100)}%`,
                          }}
                        />
                      </div>
                      <b>{p.count}</b>
                    </div>
                  ))
                ) : (
                  <p>{t("No inbounds yet.")}</p>
                )}
              </div>
            </CardContent>
          </Card>
        )}
      </section>
      {!reseller && (
        <section
          className="dashboard-runtime"
          aria-label={t("Server information", "اطلاعات سرور")}
        >
          <div>
            <span>
              <FaIcon icon="clock-rotate-left" />
              {t("Server uptime", "زمان روشن‌بودن سرور")}
            </span>
            <strong>{uptime(sys?.uptime_seconds)}</strong>
          </div>
          <div>
            <span>
              <FaIcon icon="gauge-high" />
              {t("Panel uptime", "زمان اجرای پنل")}
            </span>
            <strong>{uptime(sys?.panel_uptime_seconds)}</strong>
          </div>
          <div>
            <span>
              <FaIcon icon="microchip" />
              {t("Load average", "میانگین بار")}
            </span>
            <strong>
              {sys?.load_average?.length
                ? sys.load_average.map((v) => v.toFixed(2)).join(" / ")
                : "—"}
            </strong>
          </div>
          <div className="runtime-address">
            <span>
              <FaIcon icon="globe" />
              {t("Server address", "آدرس سرور")}
            </span>
            <strong>
              {local?.public_host || sys?.hostname
                ? showAddress
                  ? local?.public_host || sys?.hostname
                  : "••••••••"
                : "—"}
            </strong>
            {(local?.public_host || sys?.hostname) && (
              <button
                type="button"
                onClick={() => setShowAddress((v) => !v)}
                aria-label={
                  showAddress
                    ? t("Hide server address", "پنهان‌کردن آدرس سرور")
                    : t("Show server address", "نمایش آدرس سرور")
                }
                aria-pressed={showAddress}
              >
                {showAddress ? <EyeOff size={15} /> : <Eye size={15} />}
              </button>
            )}
          </div>
        </section>
      )}
      <section className={reseller ? "hidden" : "dashboard-node-section"}>
        {!reseller && (
          <>
            <Card>
              <CardHeader>
                <div>
                  <CardTitle>{t("Nodes")}</CardTitle>
                </div>
                <Badge
                  tone={
                    nodes.some((n) => n.status === "ok") ? "green" : "amber"
                  }
                >
                  {nodes.filter((n) => n.status === "ok").length} {t("online")}
                </Badge>
              </CardHeader>
              <CardContent className="p-0">
                <div className="divide-y divide-[var(--border-subtle)]">
                  {nodes.length ? (
                    nodes.slice(0, 6).map((n) => (
                      <div
                        key={n.id}
                        className="flex min-w-0 items-center gap-3 px-5 py-3.5"
                      >
                        <div className="grid h-9 w-9 shrink-0 place-items-center rounded-md border border-[var(--border)] bg-[var(--surface-2)]">
                          <Server className="h-4 w-4 text-[var(--muted-strong)]" />
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="flex min-w-0 items-center gap-2">
                            <span className="truncate text-[12px] font-semibold">
                              {n.name}
                            </span>
                            {n.is_local && (
                              <Badge tone="blue">{t("Local")}</Badge>
                            )}
                          </div>
                          <div className="mt-0.5 truncate text-[9px] text-[var(--muted)]">
                            {n.public_host} ·{" "}
                            {n.xray_version || t("Version pending")}
                          </div>
                        </div>
                        <Badge
                          tone={
                            n.status === "ok"
                              ? "green"
                              : n.status === "offline"
                                ? "red"
                                : "amber"
                          }
                        >
                          {n.status}
                        </Badge>
                      </div>
                    ))
                  ) : (
                    <div className="grid place-items-center px-5 py-12 text-center">
                      <WifiOff className="mb-3 h-5 w-5 text-[var(--muted)]" />
                      <div className="text-[12px] font-medium">
                        {t("No nodes registered")}
                      </div>
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          </>
        )}
      </section>
    </div>
  );
}
