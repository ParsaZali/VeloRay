import { useEffect, useState, type FormEvent } from "react";
import {
  Code,
  Plus,
  Pencil,
  Trash2,
  X,
  Route,
  ArrowUpRight,
  Copy,
  ShieldCheck,
  RefreshCw,
} from "lucide-react";
import { api, type NodeItem } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/common/page-header";
import { Dialog } from "@/components/common/dialog";
import { Input, Select, Textarea } from "@/components/ui/controls";
import { can, useSession } from "@/lib/session";
import { OutboundTraffic } from "@/components/outbound-traffic";
import { GeoAssets } from "@/components/geo-assets";
import {
  XrayBalancing,
  type BalancingState,
} from "@/components/xray-balancing";
import {
  configPath,
  needsProtocolSniff,
  outboundTemplate,
  parsePorts,
  routingPreset,
  setConfigPath,
  singTemplates,
  xrayTemplates,
  type NetworkConfig,
  type NetworkEntry as Entry,
} from "@/lib/network-config";

export function OutboundsPage() {
  return <NetworkPage key="outbounds" kind="outbounds" />;
}
export function RoutingPage() {
  return <NetworkPage key="routing" kind="routing_rules" />;
}
function NetworkPage({ kind }: { kind: "outbounds" | "routing_rules" }) {
  useLocale();
  const session = useSession(),
    routing = kind === "routing_rules",
    title = routing ? t("Routing") : t("Outbounds"),
    editable = can(session, `${kind}.write`);
  const [items, setItems] = useState<Entry[]>([]),
    [balancing, setBalancing] = useState<BalancingState>({
      node: 0,
      tags: [],
      outboundTags: [],
    }),
    [outbounds, setOutbounds] = useState<Entry[]>([]),
    [nodes, setNodes] = useState<NodeItem[]>([]),
    [nodeFilter, setNodeFilter] = useState(0),
    [coreFilter, setCoreFilter] = useState("xray"),
    [query, setQuery] = useState(""),
    [editor, setEditor] = useState<Entry | null>(null),
    [deleting, setDeleting] = useState<Entry | null>(null),
    [inspecting, setInspecting] = useState<NetworkConfig | null>(null),
    [text, setText] = useState("{}"),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true);
  async function load(signal?: AbortSignal) {
    setLoading(true);
    try {
      const [a, b, c] = await Promise.all([
        api<Entry[]>(`/${kind}`, { signal }),
        api<NodeItem[]>("/nodes", { signal }),
        routing ? api<Entry[]>("/outbounds", { signal }) : Promise.resolve([]),
      ]);
      if (signal?.aborted) return;
      setItems(a);
      setNodes(b);
      setOutbounds(routing ? c : a);
      setNodeFilter((current) =>
        b.some((n) => n.id === current) ? current : b[0]?.id || 0,
      );
    } catch (e) {
      if (!signal?.aborted)
        setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }
  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [kind]);
  function open(item?: Entry) {
    const core = item?.core || coreFilter;
    const d = item
      ? { ...item, config: structuredClone(item.config) }
      : {
          name: "",
          node: nodeFilter,
          core,
          enabled: true,
          tag: "",
          protocol: core === "xray" ? "freedom" : "direct",
          priority: 100,
          config: routing
            ? routingPreset(core, "custom")
            : outboundTemplate(core, core === "xray" ? "freedom" : "direct"),
        };
    setEditor(d);
    setText(JSON.stringify(d.config, null, 2));
    setError("");
  }
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!editor) return;
    setBusy(true);
    setError("");
    try {
      const config = JSON.parse(text);
      if (!config || Array.isArray(config) || typeof config !== "object")
        throw Error(t("Enter a JSON object."));
      const ports = new FormData(e.currentTarget).get("routing_ports");
      if (typeof ports === "string") {
        const parsed = parsePorts(ports);
        delete config.port;
        delete config.port_range;
        if (editor.core === "xray") {
          if (ports.trim()) config.port = ports.trim();
        } else Object.assign(config, parsed);
      }
      await api(`/${kind}/${editor.id ? editor.id + "/" : ""}`, {
        method: editor.id ? "PATCH" : "POST",
        body: JSON.stringify({
          name: editor.name,
          node: editor.node,
          core: editor.core,
          enabled: editor.enabled,
          config,
          ...(routing
            ? { priority: editor.priority }
            : { tag: editor.tag, protocol: editor.protocol }),
        }),
      });
      setEditor(null);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (!deleting) return;
    setBusy(true);
    setError("");
    try {
      await api(`/${kind}/${deleting.id}/`, { method: "DELETE" });
      setDeleting(null);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  async function toggle(item: Entry) {
    setBusy(true);
    setError("");
    try {
      await api(`/${kind}/${item.id}/`, {
        method: "PATCH",
        body: JSON.stringify({ enabled: !item.enabled }),
      });
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  const set = <K extends keyof Entry>(key: K, value: Entry[K]) =>
    setEditor((d) => (d ? { ...d, [key]: value } : d));
  let config: NetworkConfig = {},
    validJSON = true;
  try {
    const parsed = JSON.parse(text);
    if (!parsed || Array.isArray(parsed) || typeof parsed !== "object")
      validJSON = false;
    else config = parsed;
  } catch {
    validJSON = false;
  }
  const changeConfig = (next: NetworkConfig) => {
    setText(JSON.stringify(next, null, 2));
  };
  const setPath = (path: (string | number)[], value: unknown) =>
    changeConfig(setConfigPath(config, path, value));
  const node = nodes.find((n) => n.id === nodeFilter);
  const baseOutbounds: NetworkConfig[] =
    coreFilter === "singbox"
      ? [{ type: "direct", tag: "direct" }]
      : Array.isArray(node?.config_patch?.outbounds)
        ? (node.config_patch.outbounds as NetworkConfig[])
        : [
            { protocol: "freedom", tag: "direct" },
            { protocol: "blackhole", tag: "blocked" },
          ];
  const patchRules = (node?.config_patch?.routing as NetworkConfig | undefined)
    ?.rules;
  const builtins: { name: string; value: string; config: NetworkConfig }[] =
    routing
      ? [
          ...(coreFilter === "xray"
            ? [
                {
                  name: t("Panel API"),
                  value: "api → api",
                  config: {
                    type: "field",
                    inboundTag: ["api"],
                    outboundTag: "api",
                  },
                },
                ...(Array.isArray(patchRules)
                  ? patchRules
                      .filter(
                        (rule: NetworkConfig) => rule.outboundTag !== "api",
                      )
                      .map((rule: NetworkConfig, i: number) => ({
                        name: `${t("Node rule")} ${i + 1}`,
                        value: String(
                          rule.outboundTag || rule.balancerTag || "",
                        ),
                        config: rule,
                      }))
                  : []),
              ]
            : []),
          ...(coreFilter === "singbox" &&
          items.some(
            (item) =>
              item.node === nodeFilter &&
              item.core === "singbox" &&
              item.enabled &&
              needsProtocolSniff(item.config),
          )
            ? [
                {
                  name: t("Protocol detection"),
                  value: "sniff",
                  config: { action: "sniff" },
                },
              ]
            : []),
          {
            name: t("Default route"),
            value:
              coreFilter === "xray"
                ? String(baseOutbounds[0]?.tag || "")
                : "direct",
            config:
              coreFilter === "xray"
                ? { outboundTag: baseOutbounds[0]?.tag }
                : { final: "direct" },
          },
        ]
      : baseOutbounds.map((out) => ({
          name: String(out.tag),
          value: String(out.protocol || out.type),
          config: out,
        }));
  const builtinRows =
    node && !routing
      ? builtins.filter((item) =>
          `${item.name} ${item.value} ${JSON.stringify(item.config)}`
            .toLowerCase()
            .includes(query.toLowerCase()),
        )
      : [];
  const filtered = items
    .filter(
      (item) =>
        item.node === nodeFilter &&
        item.core === coreFilter &&
        `${item.name} ${item.tag || ""} ${JSON.stringify(item.config)}`
          .toLowerCase()
          .includes(query.toLowerCase()),
    )
    .sort((a, b) =>
      routing
        ? (a.priority || 0) - (b.priority || 0) || (a.id || 0) - (b.id || 0)
        : (a.id || 0) - (b.id || 0),
    );
  const xray = editor?.core === "xray",
    protocol = editor?.protocol || "",
    remote = [
      "vless",
      "vmess",
      "trojan",
      "shadowsocks",
      "socks",
      "http",
      "hysteria2",
      "tuic",
      "anytls",
      "ssh",
    ].includes(protocol),
    uuid = ["vless", "vmess", "tuic"].includes(protocol);
  const serverPath: (string | number)[] = xray
    ? ["settings", uuid ? "vnext" : "servers", 0]
    : [];
  const path = (field: string) => [...serverPath, field];
  const tags = [
    ...(xray && balancing.node === editor?.node ? balancing.outboundTags : []),
    "direct",
    ...(xray ? ["blocked"] : []),
    ...outbounds
      .filter(
        (o) => o.node === editor?.node && o.core === editor?.core && o.enabled,
      )
      .map((o) => o.tag || ""),
  ];
  const matchFields = xray
    ? [
        ["Domains", "domain"],
        ["Destination IPs", "ip"],
        ["Source IPs", "source"],
        ["Users", "user"],
        ["Inbound tags", "inboundTag"],
        ["Protocols", "protocol"],
      ]
    : [
        ["Domains", "domain_suffix"],
        ["Destination IPs", "ip_cidr"],
        ["Source IPs", "source_ip_cidr"],
        ["Users", "auth_user"],
        ["Inbound tags", "inbound"],
        ["Protocols", "protocol"],
      ];
  return (
    <div className="space-y-6">
      <PageHeader
        title={title}
        description=""
        actions={
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="icon"
              aria-label={t("Refresh")}
              disabled={loading || busy}
              onClick={() => void load()}
            >
              <RefreshCw size={15} />
            </Button>
            {editable && (
              <Button
                disabled={!nodes.length || loading || busy}
                onClick={() => open()}
              >
                <Plus size={15} />
                {routing ? t("New rule") : t("New outbound")}
              </Button>
            )}
          </div>
        }
      />
      {error && !editor && (
        <div role="alert" className="form-error">
          {error}
        </div>
      )}
      <div className="network-filters">
        <label className="label">
          {t("Node")}
          <Select
            className="input"
            value={nodeFilter}
            onChange={(e) => setNodeFilter(+e.target.value)}
          >
            {nodes.map((n) => (
              <option key={n.id} value={n.id}>
                {n.name}
              </option>
            ))}
          </Select>
        </label>
        <label className="label">
          {t("Core")}
          <Select
            className="input"
            value={coreFilter}
            onChange={(e) => setCoreFilter(e.target.value)}
          >
            <option value="xray">Xray</option>
            <option value="singbox">sing-box</option>
          </Select>
        </label>
        <label className="label">
          {t("Search")}
          <Input
            className="input"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={routing ? t("Name or match") : t("Name or tag")}
          />
        </label>
      </div>
      {node && routing && coreFilter === "xray" && session?.is_superuser && (
        <GeoAssets
          key={`geo-${node.id}`}
          node={node.id}
          onUse={
            editable
              ? (entry, kind) => {
                  const config = {
                    ...routingPreset("xray", "custom"),
                    [kind === "geoip" ? "ip" : "domain"]: [entry.reference],
                  };
                  setEditor({
                    name: entry.tag,
                    node: node.id,
                    core: "xray",
                    enabled: true,
                    priority: 100,
                    config,
                  });
                  setText(JSON.stringify(config, null, 2));
                  setError("");
                }
              : undefined
          }
        />
      )}
      {node && routing && coreFilter === "xray" && session?.is_superuser && (
        <XrayBalancing
          key={node.id}
          node={node.id}
          onGroupsChange={setBalancing}
        />
      )}
      {node && routing && (
        <div className="network-builtins">
          {builtins.map((item, i) => (
            <button
              type="button"
              className="network-system-card"
              key={i}
              onClick={() => setInspecting(item.config)}
            >
              <ShieldCheck size={18} />
              <span>
                <strong>{item.name}</strong>
                <small>{item.value}</small>
              </span>
              <Badge>{t("Built-in")}</Badge>
            </button>
          ))}
        </div>
      )}
      <Card>
        <CardHeader>
          <CardTitle>{routing ? t("Rules") : t("Outbounds")}</CardTitle>
          <Badge>{filtered.length + builtinRows.length}</Badge>
        </CardHeader>
        <CardContent className="p-0">
          <div className="table-wrap">
            <table className="data-table min-w-[650px]">
              <thead>
                <tr>
                  <th>{t("Name")}</th>
                  <th>{routing ? t("Priority") : t("Protocol")}</th>
                  <th>{routing ? t("Match") : t("Tag")}</th>
                  <th>{routing ? t("Destination") : t("Server")}</th>
                  <th>{t("Status")}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {builtinRows.map((item) => (
                  <tr
                    key={`builtin-${item.name}`}
                    className="network-builtin-row"
                  >
                    <td>
                      <button
                        type="button"
                        className="inline-flex items-center gap-2 font-semibold"
                        aria-label={`${t("Inspect")} ${item.name}`}
                        onClick={() => setInspecting(item.config)}
                      >
                        <ShieldCheck size={14} />
                        {item.name}
                      </button>
                    </td>
                    <td>{item.value}</td>
                    <td>{item.name}</td>
                    <td>—</td>
                    <td>
                      <Badge>{t("Built-in")}</Badge>
                    </td>
                    <td>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={`${t("Configuration")} ${item.name}`}
                        onClick={() => setInspecting(item.config)}
                      >
                        <Code size={14} />
                      </Button>
                    </td>
                  </tr>
                ))}
                {filtered.map((item) => (
                  <tr key={item.id}>
                    <td className="font-semibold">
                      <span className="inline-flex gap-2 items-center">
                        {routing ? (
                          <Route size={14} />
                        ) : (
                          <ArrowUpRight size={14} />
                        )}{" "}
                        {item.name}
                      </span>
                    </td>
                    <td>{routing ? item.priority : item.protocol}</td>
                    <td
                      className="max-w-[230px] truncate"
                      title={JSON.stringify(item.config)}
                    >
                      {routing
                        ? Object.entries(item.config)
                            .filter(
                              ([key]) =>
                                ![
                                  "action",
                                  "outbound",
                                  "outboundTag",
                                  "balancerTag",
                                  "type",
                                ].includes(key),
                            )
                            .map(
                              ([key, value]) =>
                                `${key}: ${Array.isArray(value) ? value.join(", ") : value}`,
                            )
                            .join(" · ") || t("All traffic")
                        : item.tag}
                    </td>
                    <td>
                      {routing
                        ? item.config.outboundTag ||
                          item.config.balancerTag ||
                          item.config.outbound ||
                          item.config.action
                        : item.config.server ||
                          item.config.settings?.servers?.[0]?.address ||
                          item.config.settings?.vnext?.[0]?.address ||
                          "—"}
                    </td>
                    <td>
                      <button
                        type="button"
                        disabled={!editable || busy}
                        onClick={() => void toggle(item)}
                        aria-label={`${item.enabled ? t("Disable") : t("Enable")} ${item.name}`}
                      >
                        <Badge tone={item.enabled ? "green" : "neutral"}>
                          {item.enabled ? t("Active") : t("Disabled")}
                        </Badge>
                      </button>
                    </td>
                    <td>
                      {editable && (
                        <div className="flex gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={t("Edit")}
                            disabled={busy}
                            onClick={() => open(item)}
                          >
                            <Pencil size={14} />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={t("Duplicate")}
                            disabled={busy}
                            onClick={() =>
                              open({
                                ...item,
                                id: undefined,
                                name: item.name + " copy",
                                tag: routing ? undefined : item.tag + "-copy",
                              })
                            }
                          >
                            <Copy size={14} />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={t("Delete")}
                            disabled={busy}
                            onClick={() => {
                              setError("");
                              setDeleting(item);
                            }}
                          >
                            <Trash2 size={14} />
                          </Button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!filtered.length && !builtinRows.length && (
              <div className="empty-state">
                {loading
                  ? t("Loading…")
                  : routing
                    ? t("No custom rules")
                    : t("No outbounds")}
              </div>
            )}
          </div>
        </CardContent>
      </Card>
      {!routing &&
        nodeFilter > 0 &&
        can(session, "nodes.read") &&
        can(session, "traffic.read") &&
        (session?.is_superuser ||
          (!session?.own_clients_only && session?.role !== "reseller")) && (
          <OutboundTraffic
            key={`${nodeFilter}:${coreFilter}`}
            node={nodeFilter}
            core={coreFilter}
          />
        )}
      {inspecting && (
        <Dialog label={t("Configuration")} onClose={() => setInspecting(null)}>
          <div className="flex items-center justify-between mb-4">
            <h3>{t("Configuration")}</h3>
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("Close")}
              onClick={() => setInspecting(null)}
            >
              <X size={17} />
            </Button>
          </div>
          <pre className="network-json" dir="ltr">
            {JSON.stringify(inspecting, null, 2)}
          </pre>
        </Dialog>
      )}
      {deleting && (
        <Dialog label={t("Delete")} onClose={() => !busy && setDeleting(null)}>
          <h3 className="text-lg font-semibold">
            {t("Delete")} {deleting.name}?
          </h3>
          {error && (
            <div role="alert" className="form-error mt-4">
              {error}
            </div>
          )}
          <div className="flex gap-2 justify-end mt-6">
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => setDeleting(null)}
            >
              {t("Cancel")}
            </Button>
            <Button disabled={busy} onClick={() => void remove()}>
              {busy ? t("Saving…") : t("Delete")}
            </Button>
          </div>
        </Dialog>
      )}
      {editor && (
        <Dialog label={title} onClose={() => !busy && setEditor(null)}>
          <div className="flex items-center justify-between mb-6">
            <h3 className="text-lg font-semibold">
              {editor.id ? t("Edit") : t("Create")} {title}
            </h3>
            <Button
              disabled={busy}
              variant="ghost"
              size="icon"
              aria-label={t("Close")}
              onClick={() => setEditor(null)}
            >
              <X size={17} />
            </Button>
          </div>
          <form onSubmit={save} className="grid gap-4 sm:grid-cols-2">
            <fieldset disabled={busy} className="contents">
              <label className="label">
                {t("Name")}
                <Input
                  className="input"
                  required
                  value={editor.name}
                  onChange={(e) => set("name", e.target.value)}
                />
              </label>
              <label className="label">
                {t("Node")}
                <Select
                  className="input"
                  value={editor.node}
                  onChange={(e) => set("node", +e.target.value)}
                >
                  {nodes.map((n) => (
                    <option key={n.id} value={n.id}>
                      {n.name}
                    </option>
                  ))}
                </Select>
              </label>
              <label className="label">
                {t("Core")}
                <Select
                  className="input"
                  value={editor.core}
                  onChange={(e) => {
                    const core = e.target.value,
                      protocol = core === "xray" ? "freedom" : "direct";
                    setEditor((d) => (d ? { ...d, core, protocol } : d));
                    changeConfig(
                      routing
                        ? routingPreset(core, "custom")
                        : outboundTemplate(core, protocol),
                    );
                  }}
                >
                  <option value="xray">Xray · Primary</option>
                  <option value="singbox">sing-box</option>
                </Select>
              </label>
              {routing ? (
                <label className="label">
                  {t("Priority")}
                  <Input
                    className="input"
                    required
                    type="number"
                    min="0"
                    max="100000"
                    value={editor.priority}
                    onChange={(e) => set("priority", +e.target.value)}
                  />
                </label>
              ) : (
                <>
                  <label className="label">
                    {t("Tag")}
                    <Input
                      className="input"
                      required
                      pattern="[A-Za-z0-9][A-Za-z0-9._-]{0,95}"
                      value={editor.tag}
                      onChange={(e) => set("tag", e.target.value)}
                    />
                  </label>
                  <label className="label">
                    {t("Protocol")}
                    <Select
                      className="input"
                      value={protocol}
                      onChange={(e) => {
                        set("protocol", e.target.value);
                        changeConfig(
                          outboundTemplate(editor.core, e.target.value),
                        );
                      }}
                    >
                      {Object.keys(xray ? xrayTemplates : singTemplates).map(
                        (p) => (
                          <option key={p} value={p}>
                            {p}
                          </option>
                        ),
                      )}
                    </Select>
                  </label>
                </>
              )}
              {validJSON && !routing && remote && (
                <>
                  <label className="label">
                    {t("Server address")}
                    <Input
                      className="input"
                      required
                      value={configPath(
                        config,
                        path(xray ? "address" : "server"),
                      )}
                      onChange={(e) =>
                        setPath(
                          path(xray ? "address" : "server"),
                          e.target.value,
                        )
                      }
                    />
                  </label>
                  <label className="label">
                    {t("Server port")}
                    <Input
                      className="input"
                      required
                      type="number"
                      min="1"
                      max="65535"
                      value={configPath(
                        config,
                        path(xray ? "port" : "server_port"),
                        443,
                      )}
                      onChange={(e) =>
                        setPath(
                          path(xray ? "port" : "server_port"),
                          +e.target.value,
                        )
                      }
                    />
                  </label>
                  {uuid && (
                    <label className="label">
                      UUID
                      <Input
                        className="input"
                        required
                        value={configPath(
                          config,
                          xray ? [...serverPath, "users", 0, "id"] : ["uuid"],
                        )}
                        onChange={(e) =>
                          setPath(
                            xray ? [...serverPath, "users", 0, "id"] : ["uuid"],
                            e.target.value,
                          )
                        }
                      />
                    </label>
                  )}
                  {[
                    "trojan",
                    "shadowsocks",
                    "hysteria2",
                    "tuic",
                    "anytls",
                    "ssh",
                  ].includes(protocol) && (
                    <label className="label">
                      {t("Password")}
                      <Input
                        className="input"
                        required
                        type="password"
                        autoComplete="new-password"
                        value={configPath(config, path("password"))}
                        onChange={(e) =>
                          setPath(path("password"), e.target.value)
                        }
                      />
                    </label>
                  )}
                  {protocol === "shadowsocks" && (
                    <label className="label">
                      {t("Encryption")}
                      <Select
                        className="input"
                        value={configPath(
                          config,
                          path("method"),
                          "aes-256-gcm",
                        )}
                        onChange={(e) =>
                          setPath(path("method"), e.target.value)
                        }
                      >
                        {[
                          "aes-256-gcm",
                          "aes-128-gcm",
                          "chacha20-ietf-poly1305",
                          "2022-blake3-aes-128-gcm",
                          "2022-blake3-aes-256-gcm",
                          "2022-blake3-chacha20-poly1305",
                        ].map((v) => (
                          <option key={v}>{v}</option>
                        ))}
                      </Select>
                    </label>
                  )}
                  {protocol !== "shadowsocks" && protocol !== "ssh" && (
                    <>
                      <label className="label">
                        {t("TLS")}
                        <Select
                          className="input"
                          value={
                            xray
                              ? config.streamSettings?.security || "none"
                              : config.tls?.enabled
                                ? "tls"
                                : "none"
                          }
                          onChange={(e) =>
                            setPath(
                              xray
                                ? ["streamSettings", "security"]
                                : ["tls", "enabled"],
                              xray ? e.target.value : e.target.value !== "none",
                            )
                          }
                        >
                          <option value="none">{t("None")}</option>
                          <option value="tls">TLS</option>
                          {xray && <option value="reality">REALITY</option>}
                        </Select>
                      </label>
                      <label className="label">
                        SNI
                        <Input
                          className="input"
                          value={configPath(
                            config,
                            xray
                              ? [
                                  "streamSettings",
                                  config.streamSettings?.security === "reality"
                                    ? "realitySettings"
                                    : "tlsSettings",
                                  "serverName",
                                ]
                              : ["tls", "server_name"],
                          )}
                          onChange={(e) =>
                            setPath(
                              xray
                                ? [
                                    "streamSettings",
                                    config.streamSettings?.security ===
                                    "reality"
                                      ? "realitySettings"
                                      : "tlsSettings",
                                    "serverName",
                                  ]
                                : ["tls", "server_name"],
                              e.target.value,
                            )
                          }
                        />
                      </label>
                    </>
                  )}
                </>
              )}

              {validJSON &&
                !routing &&
                ["socks", "http", "ssh"].includes(protocol) && (
                  <>
                    <label className="label">
                      {t("Username")}
                      <Input
                        className="input"
                        value={configPath(
                          config,
                          xray
                            ? [...serverPath, "users", 0, "user"]
                            : [protocol === "ssh" ? "user" : "username"],
                        )}
                        onChange={(e) =>
                          setPath(
                            xray
                              ? [...serverPath, "users", 0, "user"]
                              : [protocol === "ssh" ? "user" : "username"],
                            e.target.value,
                          )
                        }
                      />
                    </label>
                    {protocol !== "ssh" && (
                      <label className="label">
                        {t("Password")}
                        <Input
                          className="input"
                          type="password"
                          autoComplete="new-password"
                          value={configPath(
                            config,
                            xray
                              ? [...serverPath, "users", 0, "pass"]
                              : ["password"],
                          )}
                          onChange={(e) =>
                            setPath(
                              xray
                                ? [...serverPath, "users", 0, "pass"]
                                : ["password"],
                              e.target.value,
                            )
                          }
                        />
                      </label>
                    )}
                  </>
                )}
              {validJSON &&
                !routing &&
                ["vless", "vmess", "trojan"].includes(protocol) && (
                  <>
                    <label className="label">
                      {t("Transport")}
                      <Select
                        className="input"
                        value={
                          xray
                            ? config.streamSettings?.network || "tcp"
                            : config.transport?.type || "tcp"
                        }
                        onChange={(e) => {
                          const next = structuredClone(config);
                          if (xray) {
                            next.streamSettings ||= {};
                            next.streamSettings.network = e.target.value;
                          } else if (e.target.value === "tcp")
                            delete next.transport;
                          else
                            next.transport = {
                              ...(next.transport || {}),
                              type: e.target.value,
                            };
                          changeConfig(next);
                        }}
                      >
                        {["tcp", "ws", "grpc", "httpupgrade"].map((value) => (
                          <option key={value} value={value}>
                            {value.toUpperCase()}
                          </option>
                        ))}
                      </Select>
                    </label>
                    {(xray
                      ? config.streamSettings?.network
                      : config.transport?.type) === "ws" && (
                      <label className="label">
                        {t("Path")}
                        <Input
                          className="input"
                          value={configPath(
                            config,
                            xray
                              ? ["streamSettings", "wsSettings", "path"]
                              : ["transport", "path"],
                            "/",
                          )}
                          onChange={(e) =>
                            setPath(
                              xray
                                ? ["streamSettings", "wsSettings", "path"]
                                : ["transport", "path"],
                              e.target.value,
                            )
                          }
                        />
                      </label>
                    )}
                    {(xray
                      ? config.streamSettings?.network
                      : config.transport?.type) === "grpc" && (
                      <label className="label">
                        {t("Service name")}
                        <Input
                          className="input"
                          value={configPath(
                            config,
                            xray
                              ? [
                                  "streamSettings",
                                  "grpcSettings",
                                  "serviceName",
                                ]
                              : ["transport", "service_name"],
                          )}
                          onChange={(e) =>
                            setPath(
                              xray
                                ? [
                                    "streamSettings",
                                    "grpcSettings",
                                    "serviceName",
                                  ]
                                : ["transport", "service_name"],
                              e.target.value,
                            )
                          }
                        />
                      </label>
                    )}
                    {xray && config.streamSettings?.security === "reality" && (
                      <>
                        <label className="label">
                          {t("Public key")}
                          <Input
                            className="input"
                            required
                            value={configPath(config, [
                              "streamSettings",
                              "realitySettings",
                              "password",
                            ])}
                            onChange={(e) =>
                              setPath(
                                [
                                  "streamSettings",
                                  "realitySettings",
                                  "password",
                                ],
                                e.target.value,
                              )
                            }
                          />
                        </label>
                        <label className="label">
                          {t("Short ID")}
                          <Input
                            className="input"
                            value={configPath(config, [
                              "streamSettings",
                              "realitySettings",
                              "shortId",
                            ])}
                            onChange={(e) =>
                              setPath(
                                [
                                  "streamSettings",
                                  "realitySettings",
                                  "shortId",
                                ],
                                e.target.value,
                              )
                            }
                          />
                        </label>
                      </>
                    )}
                  </>
                )}
              {validJSON &&
                !routing &&
                ["selector", "urltest"].includes(protocol) && (
                  <label className="label sm:col-span-2">
                    {t("Outbound tags")}
                    <Textarea
                      className="input min-h-[76px]"
                      value={(config.outbounds || []).join("\n")}
                      onChange={(e) =>
                        setPath(
                          ["outbounds"],
                          e.target.value.split(/\r?\n/).filter((v) => v.trim()),
                        )
                      }
                    />
                  </label>
                )}
              {validJSON && !routing && (
                <>
                  {![
                    "selector",
                    "urltest",
                    "blackhole",
                    "dns",
                    "loopback",
                    ...(!xray ? ["direct"] : []),
                  ].includes(protocol) && (
                    <label className="label">
                      {t("Next hop (optional)")}
                      <Select
                        value={
                          xray
                            ? configPath(
                                config,
                                ["streamSettings", "sockopt", "dialerProxy"],
                                configPath(config, ["proxySettings", "tag"]),
                              )
                            : config.detour || ""
                        }
                        onChange={(e) => {
                          const next = xray
                            ? setConfigPath(
                                config,
                                ["streamSettings", "sockopt", "dialerProxy"],
                                e.target.value,
                              )
                            : { ...config, detour: e.target.value };
                          if (xray && next.proxySettings?.tag) {
                            delete next.proxySettings.tag;
                            if (!Object.keys(next.proxySettings).length)
                              delete next.proxySettings;
                          }
                          changeConfig(next);
                        }}
                      >
                        <option value="">{t("Connect directly")}</option>
                        {Array.from(new Set(tags))
                          .filter(
                            (tag) => tag !== editor.tag && tag !== "blocked",
                          )
                          .map((tag) => (
                            <option key={tag} value={tag}>
                              {tag}
                            </option>
                          ))}
                      </Select>
                      <span>
                        {t(
                          "Dial this outbound through another outbound. Recursive chains are rejected.",
                        )}
                      </span>
                    </label>
                  )}
                  {!xray && protocol === "selector" && (
                    <label className="label">
                      {t("Default candidate")}
                      <Select
                        value={config.default || ""}
                        onChange={(e) => setPath(["default"], e.target.value)}
                      >
                        <option value="">{t("First candidate")}</option>
                        {(config.outbounds || []).map((tag: string) => (
                          <option key={tag} value={tag}>
                            {tag}
                          </option>
                        ))}
                      </Select>
                    </label>
                  )}
                  {!xray && protocol === "urltest" && (
                    <>
                      <label className="label sm:col-span-2">
                        {t("Probe URL")}
                        <Input
                          type="url"
                          value={config.url || ""}
                          placeholder="https://www.gstatic.com/generate_204"
                          onChange={(e) => setPath(["url"], e.target.value)}
                        />
                      </label>
                      <label className="label">
                        {t("Probe interval")}
                        <Input
                          value={config.interval || ""}
                          placeholder="3m"
                          onChange={(e) =>
                            setPath(["interval"], e.target.value)
                          }
                        />
                      </label>
                      <label className="label">
                        {t("Latency tolerance (ms)")}
                        <Input
                          type="number"
                          min={0}
                          max={60000}
                          value={config.tolerance ?? 50}
                          onChange={(e) =>
                            setPath(["tolerance"], +e.target.value)
                          }
                        />
                      </label>
                      <label className="label">
                        {t("Probe idle timeout")}
                        <Input
                          value={config.idle_timeout || ""}
                          placeholder="30m"
                          onChange={(e) =>
                            setPath(["idle_timeout"], e.target.value)
                          }
                        />
                      </label>
                    </>
                  )}
                  {!xray && ["selector", "urltest"].includes(protocol) && (
                    <label className="flex items-center gap-2 text-[12px] sm:col-span-2">
                      <input
                        type="checkbox"
                        checked={!!config.interrupt_exist_connections}
                        onChange={(e) =>
                          setPath(
                            ["interrupt_exist_connections"],
                            e.target.checked,
                          )
                        }
                      />
                      {t("Interrupt existing connections when switching")}
                    </label>
                  )}
                </>
              )}
              {validJSON && routing && (
                <>
                  <div className="sm:col-span-2 network-presets">
                    {[
                      "private",
                      "bittorrent",
                      "https",
                      ...(xray ? ["ads"] : []),
                    ].map((preset) => (
                      <Button
                        key={preset}
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() =>
                          changeConfig(routingPreset(editor.core, preset))
                        }
                      >
                        {t(
                          {
                            private: "Block private IPs",
                            bittorrent: "Block BitTorrent",
                            https: "Direct HTTPS",
                            ads: "Block ads",
                          }[
                            preset as "private" | "bittorrent" | "https" | "ads"
                          ] || preset,
                        )}
                      </Button>
                    ))}
                  </div>
                  <label className="label">
                    {t("Destination")}
                    <Select
                      className="input"
                      value={
                        xray
                          ? config.balancerTag
                            ? `balancer:${config.balancerTag}`
                            : config.outboundTag || "direct"
                          : config.action === "reject"
                            ? "__reject"
                            : config.outbound || "direct"
                      }
                      onChange={(e) => {
                        const next = { ...config };
                        if (xray) {
                          delete next.outboundTag;
                          delete next.balancerTag;
                          if (e.target.value.startsWith("balancer:"))
                            next.balancerTag = e.target.value.slice(9);
                          else next.outboundTag = e.target.value;
                        } else if (e.target.value === "__reject") {
                          next.action = "reject";
                          delete next.outbound;
                        } else {
                          next.action = "route";
                          next.outbound = e.target.value;
                        }
                        changeConfig(next);
                      }}
                    >
                      {Array.from(new Set(tags)).map((tag) => (
                        <option key={tag} value={tag}>
                          {tag}
                        </option>
                      ))}
                      {xray &&
                        balancing.node === editor.node &&
                        balancing.tags.map((tag) => (
                          <option
                            key={`balancer:${tag}`}
                            value={`balancer:${tag}`}
                          >
                            {t("Balancer")}: {tag}
                          </option>
                        ))}
                      {xray &&
                        config.balancerTag &&
                        !(
                          balancing.node === editor.node &&
                          balancing.tags.includes(config.balancerTag)
                        ) && (
                          <option value={`balancer:${config.balancerTag}`}>
                            {t("Balancer")}: {config.balancerTag}
                          </option>
                        )}
                      {!xray && <option value="__reject">{t("Block")}</option>}
                    </Select>
                  </label>
                  <label className="label">
                    {t("Network")}
                    <Select
                      className="input"
                      value={
                        Array.isArray(config.network)
                          ? config.network.join(",")
                          : config.network || ""
                      }
                      onChange={(e) => {
                        const next = { ...config };
                        if (e.target.value)
                          next.network = xray
                            ? e.target.value
                            : e.target.value.split(",");
                        else delete next.network;
                        changeConfig(next);
                      }}
                    >
                      <option value="">{t("Any")}</option>
                      <option value="tcp">TCP</option>
                      <option value="udp">UDP</option>
                      <option value="tcp,udp">TCP + UDP</option>
                    </Select>
                  </label>
                  {matchFields.map(([label, key]) => (
                    <label className="label" key={key}>
                      {t(label)}
                      <Textarea
                        className="input min-h-[76px]"
                        dir="ltr"
                        placeholder={
                          key === "domain"
                            ? "domain:example.com"
                            : key === "domain_suffix"
                              ? "example.com"
                              : ""
                        }
                        value={
                          Array.isArray(config[key])
                            ? config[key].join("\n")
                            : ""
                        }
                        onChange={(e) => {
                          const next = { ...config },
                            values = e.target.value
                              .split(/\r?\n/)
                              .filter((v) => v.trim());
                          if (values.length) next[key] = values;
                          else delete next[key];
                          changeConfig(next);
                        }}
                      />
                    </label>
                  ))}
                  <label className="label">
                    {t("Ports")}
                    <Input
                      name="routing_ports"
                      className="input"
                      dir="ltr"
                      defaultValue={
                        xray
                          ? config.port || ""
                          : [
                              ...(config.port || []),
                              ...(config.port_range || []),
                            ].join(", ")
                      }
                      key={`${editor.id}-${editor.core}-${JSON.stringify(config.port)}-${JSON.stringify(config.port_range)}`}
                      placeholder="443, 8000-9000"
                      onBlur={(e) => {
                        try {
                          const next = { ...config };
                          delete next.port;
                          delete next.port_range;
                          if (xray) {
                            if (e.target.value.trim())
                              next.port = e.target.value.trim();
                          } else
                            Object.assign(next, parsePorts(e.target.value));
                          changeConfig(next);
                          setError("");
                        } catch (e) {
                          setError(
                            e instanceof Error ? t(e.message) : String(e),
                          );
                        }
                      }}
                    />
                  </label>
                </>
              )}
              <details className="sm:col-span-2" open={!validJSON}>
                <summary className="network-advanced">
                  {t("Advanced configuration")}
                </summary>
                <Textarea
                  aria-label={t("Configuration")}
                  className="input min-h-[220px] font-mono text-[11px] leading-6 mt-3"
                  spellCheck={false}
                  dir="ltr"
                  value={text}
                  onChange={(e) => setText(e.target.value)}
                />
              </details>
              <label className="check-row">
                <Input
                  type="checkbox"
                  checked={editor.enabled}
                  onChange={(e) => set("enabled", e.target.checked)}
                />
                {t("Enabled")}
              </label>
              {error && (
                <div role="alert" className="form-error sm:col-span-2">
                  {error}
                </div>
              )}
              <div className="sm:col-span-2 flex justify-end gap-2">
                <Button
                  variant="outline"
                  type="button"
                  onClick={() => setEditor(null)}
                >
                  {t("Cancel")}
                </Button>
                <Button type="submit">
                  {busy ? t("Saving…") : t("Save & deploy")}
                </Button>
              </div>
            </fieldset>
          </form>
        </Dialog>
      )}
    </div>
  );
}
