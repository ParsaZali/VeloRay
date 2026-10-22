import { CoreManager } from "@/components/core-manager";
import { Textarea } from "@/components/ui/controls";
import { Select } from "@/components/ui/controls";
import { t } from "@/lib/i18n";
import { useLocale, tx } from "@/lib/i18n";
import { useEffect, useMemo, useRef, useState } from "react";
import { api, type NodeItem, type PortReport } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/common/page-header";
import { FaIcon } from "@/components/ui/fa-icon";

type Preview = { node: string; config: Record<string, unknown> };
type XrayStatus = {
  installed: boolean;
  running: boolean;
  version: string;
  state?: string;
};

export function XrayPage() {
  useLocale();
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [nodeId, setNodeId] = useState("");
  const [preview, setPreview] = useState<Preview | null>(null);
  const [status, setStatus] = useState<XrayStatus | null>(null);
  const [secondary, setSecondary] = useState<XrayStatus | null>(null);
  const [ports, setPorts] = useState<PortReport | null>(null);
  const [patch, setPatch] = useState("{}");
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);
  const refreshSequence = useRef(0);

  async function loadNodes() {
    const list = await api<NodeItem[]>("/nodes/");
    setNodes(list);
    const preferred = list.find((x) => x.is_local) || list[0];
    if (preferred && !nodeId) setNodeId(String(preferred.id));
  }
  async function refresh(id = nodeId) {
    if (!id) return;
    const sequence = ++refreshSequence.current;
    setStatus(null);
    setPreview(null);
    setPorts(null);
    setBusy(true);
    try {
      const [p, s, listeners] = await Promise.all([
        api<Preview>(`/nodes/${id}/config-preview/`),
        api<XrayStatus>(`/nodes/${id}/xray-status/`),
        api<PortReport>(`/nodes/${id}/ports/`).catch((e: Error) => ({
          ports: [],
          error: e.message,
        })),
      ]);
      if (sequence !== refreshSequence.current) return;
      setPreview(p);
      setStatus(s);
      setSecondary(
        await api<XrayStatus>(`/nodes/${id}/singbox-status/`).catch(() => null),
      );
      setPorts(listeners);
      const n = nodes.find((x) => String(x.id) === id);
      if (n) setPatch(JSON.stringify(n.config_patch || {}, null, 2));
      setMsg("");
    } catch (e: any) {
      if (sequence === refreshSequence.current) setMsg(e.message);
    } finally {
      if (sequence === refreshSequence.current) setBusy(false);
    }
  }
  useEffect(() => {
    loadNodes().catch((e) => setMsg(e.message));
  }, []);
  useEffect(() => {
    if (nodeId) void refresh(nodeId);
    return () => {
      refreshSequence.current++;
    };
  }, [nodeId, nodes.length]);
  const selected = useMemo(
    () => nodes.find((x) => String(x.id) === nodeId),
    [nodes, nodeId],
  );
  async function savePatch() {
    if (!selected) return;
    setBusy(true);
    try {
      let parsed: Record<string, unknown>;
      try {
        parsed = JSON.parse(patch || "{}");
      } catch {
        throw new Error(t("Advanced patch must be valid JSON."));
      }
      if (
        Array.isArray(parsed) ||
        parsed === null ||
        typeof parsed !== "object"
      )
        throw new Error(t("Advanced patch must be a JSON object."));
      await api(`/nodes/${selected.id}/`, {
        method: "PATCH",
        body: JSON.stringify({ config_patch: parsed }),
      });
      await loadNodes();
      await refresh(String(selected.id));
      setMsg(t("Advanced patch saved, validated and deployed."));
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function deploy() {
    if (!selected) return;
    setBusy(true);
    try {
      await api(`/nodes/${selected.id}/deploy/`, { method: "POST" });
      await refresh();
      setMsg(t("Configuration deployed."));
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function restart() {
    if (!selected) return;
    if (
      !confirm(
        tx(
          "Restart Xray on {0}? Active connections may briefly reconnect.",
          selected.name,
        ),
      )
    )
      return;
    setBusy(true);
    try {
      await api(`/nodes/${selected.id}/xray-restart/`, { method: "POST" });
      await refresh();
      setMsg(t("Xray restarted."));
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function validateConfig() {
    if (!selected) return;
    setBusy(true);
    try {
      await api(`/nodes/${selected.id}/config-validate/`, { method: "POST" });
      setMsg(t("Configuration is valid. The running service was not changed."));
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function copyConfig() {
    if (!preview) return;
    try {
      await navigator.clipboard.writeText(
        JSON.stringify(preview.config, null, 2),
      );
      setMsg(t("Configuration copied."));
    } catch {
      setMsg(
        t("Clipboard is unavailable. Download the configuration instead."),
      );
    }
  }
  function downloadConfig() {
    if (!preview) return;
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(preview.config, null, 2)], {
        type: "application/json",
      }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = tx("veloray-node-{0}.json", nodeId);
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  function downloadDiagnostics() {
    if (!selected) return;
    const data = {
      version: "1.0",
      at: new Date().toISOString(),
      node: { id: selected.id, name: selected.name },
      runtime: status,
      listeners: ports,
    };
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(data, null, 2)], { type: "application/json" }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = tx("veloray-diagnostics-{0}.json", selected.id);
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Cores", "هسته‌ها")}
        description={t(
          "Service health, connection configuration and deployment.",
        )}
        actions={
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              onClick={() => refresh()}
              disabled={busy || !nodeId}
            >
              <FaIcon icon="rotate" />
              {t("Refresh")}
            </Button>
            <Button
              variant="outline"
              onClick={validateConfig}
              disabled={busy || !nodeId}
            >
              <FaIcon icon="shield-halved" />
              {t("Validate")}
            </Button>
            <Button onClick={deploy} disabled={busy || !nodeId}>
              <FaIcon icon="rocket" />
              {t("Deploy")}
            </Button>
            <Button
              variant="outline"
              onClick={downloadDiagnostics}
              disabled={busy || !status}
            >
              <FaIcon icon="download" /> {t("Diagnostics")}
            </Button>
          </div>
        }
      />
      <div className="grid gap-3 sm:grid-cols-2">
        {[
          ["Xray · Primary", status],
          ["sing-box", secondary],
        ].map(([label, state]) => {
          const st = state as XrayStatus | null;
          return (
            <Card key={String(label)}>
              <CardContent className="flex justify-between items-center p-5">
                <div>
                  <h3 className="text-sm font-semibold">{String(label)}</h3>
                  <p className="text-xs text-[var(--muted)] mt-1">
                    {st?.version || "—"}
                  </p>
                </div>
                <div className="flex items-center gap-3">
                  <Badge tone={st?.running ? "green" : "neutral"}>
                    {st?.running ? t("Active") : t("Stopped")}
                  </Badge>
                  {label === "sing-box" && st?.installed && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={busy}
                      onClick={async () => {
                        setBusy(true);
                        try {
                          await api(`/nodes/${nodeId}/singbox-restart`, {
                            method: "POST",
                          });
                          await refresh();
                        } catch (e: any) {
                          setMsg(e.message);
                        } finally {
                          setBusy(false);
                        }
                      }}
                    >
                      {t("Restart")}
                    </Button>
                  )}
                </div>
              </CardContent>
            </Card>
          );
        })}
      </div>
      {nodeId && <CoreManager key={nodeId} nodeId={nodeId} />}
      {msg && (
        <div
          role="status"
          aria-live="polite"
          className="rounded-xl border border-[var(--border)] bg-[var(--card)] px-4 py-3 text-[12px] text-[var(--muted-strong)]"
        >
          {msg}
        </div>
      )}
      {status && !status.running && (
        <div
          role="alert"
          className="rounded-xl border border-red-500/25 bg-red-500/5 p-4 text-[12px] leading-6"
        >
          <div className="font-semibold text-red-500">
            {t("Xray is")} {status.installed ? "stopped" : t("not installed")}{" "}
            {t("on")} {selected?.name}.
          </div>
          <p className="text-[var(--muted-strong)]">
            {t(
              "Review the listener checks below. Resolve conflicting ports in Inbounds, then deploy or restart Xray.",
            )}
          </p>
        </div>
      )}
      <Card>
        <CardHeader>
          <div>
            <CardTitle>{t("Listener checks")}</CardTitle>
          </div>
          <a
            href="/inbounds"
            className="text-[11px] font-semibold text-[var(--brand)]"
          >
            {t("Manage inbounds →")}
          </a>
        </CardHeader>
        <CardContent>
          {ports?.error ? (
            <p role="alert" className="text-[12px] text-red-500">
              {ports.error}
            </p>
          ) : !ports ? (
            <p className="text-[12px] text-[var(--muted)]">
              {t("Checking listeners…")}
            </p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-[11px]">
                <thead className="text-[var(--muted)]">
                  <tr>
                    <th className="pb-3">{t("Inbound")}</th>
                    <th className="pb-3">{t("Listener")}</th>
                    <th className="pb-3">{t("Status")}</th>
                    <th className="pb-3">{t("Owner")}</th>
                  </tr>
                </thead>
                <tbody>
                  {(ports.ports || []).map((p, index) => (
                    <tr
                      key={`${p.tag}-${p.network}-${p.port}-${index}`}
                      className="border-t border-[var(--border)]"
                    >
                      <td className="py-3 pr-4">{p.tag}</td>
                      <td className="py-3 pr-4 font-mono whitespace-nowrap">
                        {p.network.toUpperCase()}{" "}
                        {p.listen.includes(":") ? `[${p.listen}]` : p.listen}:
                        {p.port}
                      </td>
                      <td className="py-3 pr-4">
                        <span
                          className={
                            p.state === "conflict"
                              ? "text-red-500"
                              : p.state === "xray"
                                ? "text-[var(--brand)]"
                                : "text-[var(--muted-strong)]"
                          }
                        >
                          {p.state === "conflict"
                            ? t("Conflict")
                            : p.state === "xray"
                              ? t("In use by Xray")
                              : t("Available")}
                        </span>
                      </td>
                      <td className="py-3 break-all text-[var(--muted)]">
                        {p.owner || "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {(ports.ports || []).length === 0 && (
                <p className="text-[12px] text-[var(--muted)]">
                  {t("No configured IP listeners.")}
                </p>
              )}
            </div>
          )}
        </CardContent>
      </Card>
      <div className="grid gap-4 xl:grid-cols-[360px_1fr]">
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <div>
                <CardTitle>{t("Runtime")}</CardTitle>
              </div>
              <FaIcon icon="microchip" className="text-[var(--muted)]" />
            </CardHeader>
            <CardContent className="space-y-4">
              <label className="label">
                {t("Node")}
                <Select
                  className="input"
                  value={nodeId}
                  onChange={(e) => setNodeId(e.target.value)}
                >
                  {nodes.map((n) => (
                    <option key={n.id} value={n.id}>
                      {n.is_local ? "★ " : ""}
                      {n.name}
                    </option>
                  ))}
                </Select>
              </label>
              <div className="grid grid-cols-2 gap-2">
                <div className="rounded-xl border border-[var(--border)] bg-[var(--surface-2)] p-3">
                  <div className="text-[9px] uppercase tracking-wider text-[var(--muted)]">
                    {t("Service")}
                  </div>
                  <div className="mt-2">
                    <Badge
                      tone={
                        status ? (status.running ? "green" : "red") : "neutral"
                      }
                    >
                      {status
                        ? status.running
                          ? t("Running")
                          : t("Stopped")
                        : "Checking…"}
                    </Badge>
                  </div>
                </div>
                <div className="rounded-xl border border-[var(--border)] bg-[var(--surface-2)] p-3">
                  <div className="text-[9px] uppercase tracking-wider text-[var(--muted)]">
                    {t("Version")}
                  </div>
                  <div className="mt-2 truncate text-[11px] font-semibold">
                    {status?.version || "—"}
                  </div>
                </div>
              </div>
              <Button
                variant="outline"
                className="w-full"
                onClick={restart}
                disabled={!selected || busy}
              >
                <FaIcon icon="power-off" />
                {t("Restart Xray")}
              </Button>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <div>
                <CardTitle>{t("Advanced merge patch")}</CardTitle>
              </div>
              <FaIcon icon="code" className="text-[var(--info)]" />
            </CardHeader>
            <CardContent>
              <Textarea
                className="input mono min-h-[280px] text-[10px] leading-5"
                spellCheck={false}
                value={patch}
                onChange={(e) => setPatch(e.target.value)}
              />
              <div className="mt-3 rounded-lg border border-amber-500/15 bg-amber-500/5 px-3 py-2 text-[10px] leading-5 text-[var(--muted-strong)]">
                <FaIcon
                  icon="triangle-exclamation"
                  className="mr-1 text-amber-500"
                />
                {t(
                  "The patch is merged into VeloRay's generated config. The local stats API and traffic counters stay enabled.",
                )}
              </div>
              <Button
                className="mt-3 w-full"
                onClick={savePatch}
                disabled={!selected || busy}
              >
                <FaIcon icon="floppy-disk" />
                {t("Save, validate & deploy")}
              </Button>
            </CardContent>
          </Card>
        </div>
        <Card>
          <CardHeader>
            <div>
              <CardTitle>{t("Generated config preview")}</CardTitle>
            </div>
            <div className="flex flex-wrap gap-1">
              <Button
                variant="ghost"
                size="sm"
                onClick={copyConfig}
                disabled={!preview}
              >
                <FaIcon icon="copy" family="regular" />
                {t("Copy")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={downloadConfig}
                disabled={!preview}
              >
                <FaIcon icon="download" />
                {t("Download")}
              </Button>
            </div>
          </CardHeader>
          <CardContent className="p-0">
            <pre className="m-0 max-h-[760px] overflow-auto p-5 text-[10px] leading-5 text-[var(--muted-strong)]">
              <code>
                {preview
                  ? JSON.stringify(preview.config, null, 2)
                  : t("Select a node to preview configuration.")}
              </code>
            </pre>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
