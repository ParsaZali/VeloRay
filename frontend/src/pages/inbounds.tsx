import { Textarea } from "@/components/ui/controls";
import { SNIScanner } from "@/components/sni-scanner";
import {
  InboundFallbacks,
  type FallbackEntry,
} from "@/components/inbound-fallbacks";
import { Select, Input } from "@/components/ui/controls";
import { t } from "@/lib/i18n";
import { useLocale, tx } from "@/lib/i18n";
import { Dialog } from "@/components/common/dialog";
import { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";
import {
  Copy,
  KeyRound,
  Pause,
  Pencil,
  Play,
  Plus,
  Rocket,
  Trash2,
  X,
} from "lucide-react";
import {
  api,
  type InboundItem,
  type NodeItem,
  type PortReport,
} from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/common/page-header";
import { FaIcon } from "@/components/ui/fa-icon";
import { cn } from "@/lib/cn";

type FormState = {
  fallbacks: FallbackEntry[];
  core: string;
  node: string;
  name: string;
  listen: string;
  port: string;
  protocol: string;
  transport: string;
  security: string;
  flow: string;
  path: string;
  host_header: string;
  service_name: string;
  tls_server_name: string;
  tls_cert_file: string;
  tls_key_file: string;
  reality_dest: string;
  reality_server_name: string;
  reality_private_key: string;
  reality_public_key: string;
  reality_short_id: string;
  wg_secret_key: string;
  wg_public_key: string;
  wg_mtu: string;
  wg_dns: string;
  socks_udp: boolean;
  ss_method: string;
  awg: Record<string, string>;
  mtproto_host: string;
  st_handshake: string;
  st_port: string;
  st_psk: string;
  hy_udp_idle: string;
  hy_masquerade: string;
  tunnel_network: string;
  tunnel_address: string;
  tunnel_port: string;
  tunnel_redirect: boolean;
  tun_name: string;
  tun_mtu: string;
  tun_gateway: string;
  tun_dns: string;
  tun_routes: string;
  tun_interface: string;
  stream_json: string;
};
const fresh = (node = ""): FormState => ({
  fallbacks: [],
  node,
  core: "xray",
  name: t("Main"),
  listen: "0.0.0.0",
  port: "443",
  protocol: "vless",
  transport: "raw",
  security: "reality",
  flow: "xtls-rprx-vision",
  path: "/",
  host_header: "",
  service_name: "",
  tls_server_name: "",
  tls_cert_file: "",
  tls_key_file: "",
  reality_dest: "www.cloudflare.com:443",
  reality_server_name: "www.cloudflare.com",
  reality_private_key: "",
  reality_public_key: "",
  reality_short_id: "",
  wg_secret_key: "",
  wg_public_key: "",
  wg_mtu: "1420",
  wg_dns: "1.1.1.1",
  socks_udp: true,
  ss_method: "aes-256-gcm",
  awg: {
    jc: "4",
    jmin: "40",
    jmax: "70",
    s1: "32",
    s2: "64",
    s3: "8",
    s4: "16",
    h1: "123456789",
    h2: "234567890",
    h3: "345678901",
    h4: "456789012",
  },
  mtproto_host: "www.cloudflare.com",
  st_handshake: "",
  st_port: "443",
  st_psk: "",
  hy_udp_idle: "60",
  hy_masquerade: "",
  tunnel_network: "tcp",
  tunnel_address: "localhost",
  tunnel_port: "0",
  tunnel_redirect: false,
  tun_name: "veloray-tun0",
  tun_mtu: "1500",
  tun_gateway: "10.66.0.1/16",
  tun_dns: "1.1.1.1,8.8.8.8",
  tun_routes: "",
  tun_interface: "auto",
  stream_json: "{}",
});
function Field({
  label,
  children,
  className = "",
}: {
  label: string;
  children: ReactNode;
  className?: string;
}) {
  useLocale();
  return (
    <label className={`label ${className}`}>
      {t(label)}
      {children}
    </label>
  );
}
function Choice({
  value,
  label,
  active,
  disabled,
  onClick,
}: {
  value: string;
  label: string;
  active: boolean;
  disabled?: boolean;
  onClick: (v: string) => void;
}) {
  useLocale();
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={() => onClick(value)}
      className={cn(
        "min-h-10 rounded-lg border px-2.5 text-[10px] font-semibold transition",
        active
          ? "border-[var(--brand)] bg-[var(--surface-2)] text-[var(--brand)] shadow-[0_0_0_3px_var(--ring)]"
          : "border-[var(--border)] bg-[var(--surface-2)] text-[var(--muted-strong)] hover:border-[var(--border-strong)] hover:text-[var(--fg)]",
        "disabled:cursor-not-allowed disabled:opacity-35",
      )}
    >
      {t(label)}
    </button>
  );
}

const protocolOptions = [
  ["vless", "VLESS", "bolt"],
  ["vmess", "VMess", "v"],
  ["trojan", "Trojan", "shield"],
  ["shadowsocks", "Shadowsocks", "ghost"],
  ["hysteria", "Hysteria2", "gauge-high"],
  ["tuic", "TUIC v5", "bolt"],
  ["naive", "Naive", "globe"],
  ["snell", "Snell v6", "shield"],
  ["shadowtls", "ShadowTLS v3", "shield"],
  ["mtproto", "MTProto · Telegram", "paper-plane"],
  ["amneziawg", "AmneziaWG", "link"],
  ["wireguard", "WireGuard", "link"],
  ["http", "HTTP Proxy", "globe"],
  ["socks", "SOCKS5", "socks"],
  ["tunnel", "Tunnel", "arrows-left-right-to-line"],
  ["tun", "TUN", "network-wired"],
] as const;
const transportOptions = [
  ["raw", "RAW / TCP"],
  ["ws", "WebSocket"],
  ["grpc", "gRPC"],
  ["xhttp", "XHTTP"],
  ["httpupgrade", "HTTPUpgrade"],
  ["mkcp", "mKCP"],
] as const;

export function InboundsPage() {
  useLocale();
  const [nodes, setNodes] = useState<NodeItem[]>([]),
    [items, setItems] = useState<InboundItem[]>([]),
    [form, setForm] = useState<FormState>(fresh()),
    [msg, setMsg] = useState(""),
    [busy, setBusy] = useState(false),
    [editing, setEditing] = useState<InboundItem | null>(null);
  const [cloning, setCloning] = useState<InboundItem | null>(null),
    [cloneNode, setCloneNode] = useState(""),
    [clonePort, setClonePort] = useState(""),
    [cloneName, setCloneName] = useState(""),
    [cloneBusy, setCloneBusy] = useState(false);
  const [portResult, setPortResult] = useState("");
  const [portBusy, setPortBusy] = useState(false);
  const [portSignature, setPortSignature] = useState("");
  const signature = JSON.stringify([
    editing?.id || 0,
    form.node,
    form.listen,
    form.port,
    form.protocol,
    form.transport,
    form.socks_udp,
    form.tunnel_network,
  ]);
  function preset(value: string) {
    if (!value) return;
    const next = fresh(form.node);
    next.core = form.core;
    if (value === "reality") {
      next.name = t("VLESS REALITY");
      next.port = "2053";
    } else if (value === "websocket") {
      next.name = t("VLESS WebSocket");
      next.port = "2083";
      next.transport = "ws";
      next.security = "tls";
      next.flow = "";
      next.path = "/vpn";
    } else if (value === "trojan") {
      next.name = t("Trojan TLS");
      next.port = "2087";
      next.protocol = "trojan";
      next.security = "tls";
      next.flow = "";
    } else if (value === "wireguard") {
      next.name = t("WireGuard");
      next.port = "51820";
      next.protocol = "wireguard";
      next.security = "none";
      next.flow = "";
    } else if (value === "hysteria") {
      next.name = "Hysteria2";
      next.port = "4443";
      next.protocol = "hysteria";
      next.transport = "hysteria";
      next.security = "tls";
      next.flow = "";
    }
    setForm(next);
    setPortResult("");
    setMsg(
      next.security === "tls"
        ? t("Set the server name and certificate paths, then check the port.")
        : t("Generate the server keypair, then check the port."),
    );
  }
  async function checkPort() {
    const snapshot = signature;
    setPortBusy(true);
    setPortSignature(snapshot);
    try {
      const result = await api<PortReport>(
        `/nodes/${form.node}/check-listener/`,
        {
          method: "POST",
          body: JSON.stringify({ ...payload(), id: editing?.id || 0 }),
        },
      );
      const conflicts = result.ports.filter((p) => p.state === "conflict");
      setPortResult(
        conflicts.length
          ? conflicts
              .map(
                (p) =>
                  `${p.network.toUpperCase()} :${p.port} — ${p.owner || "occupied"}`,
              )
              .join(" · ")
          : t(
              "Port check passed. Availability is checked again before deployment.",
            ),
      );
    } catch (error: unknown) {
      setPortResult(
        error instanceof Error ? error.message : t("Port check failed."),
      );
    } finally {
      setPortBusy(false);
    }
  }
  const load = async () => {
    const [n, i] = await Promise.all([
      api<NodeItem[]>("/nodes/"),
      api<InboundItem[]>("/inbounds/"),
    ]);
    setNodes(n);
    setItems(i);
    setForm((f) =>
      f.node
        ? f
        : { ...f, node: String((n.find((x) => x.is_local) || n[0])?.id || "") },
    );
  };
  useEffect(() => {
    load().catch((e) => setMsg(e.message));
  }, []);
  function set(k: keyof FormState, v: any) {
    setForm((f) => {
      const n: any = { ...f, [k]: v };
      if (k === "protocol") {
        if (["mtproto", "amneziawg"].includes(v)) {
          n.core = "native";
          if (v === "amneziawg") n.wg_mtu = "1280";
          n.transport = "raw";
          n.security = "none";
          n.flow = "";
        } else if (["tuic", "naive", "snell", "shadowtls"].includes(v)) {
          n.core = "singbox";
          n.transport = "raw";
          n.security = ["snell", "shadowtls"].includes(v) ? "none" : "tls";
          n.flow = "";
        } else if (v === "shadowsocks" || v === "socks") {
          n.transport = "raw";
          n.security = "none";
          n.flow = "";
        } else if (v === "wireguard") {
          n.transport = "raw";
          n.security = "none";
          n.flow = "";
        } else if (v === "tunnel") {
          n.transport = "raw";
          n.security = "none";
          n.flow = "";
          if (n.port === "0") n.port = "443";
        } else if (v === "tun") {
          n.transport = "raw";
          n.security = "none";
          n.flow = "";
          n.port = "0";
        } else if (v === "hysteria") {
          n.transport = "hysteria";
          n.security = "tls";
          n.flow = "";
        } else if (v !== "vless" && n.security === "reality") {
          n.security = "none";
          n.flow = "";
        }
      }
      if (k === "transport" && v !== "raw") n.flow = "";
      if (k === "security" && v !== "reality") n.flow = "";
      if (
        n.protocol === "vless" &&
        n.transport === "raw" &&
        n.security === "reality" &&
        !n.flow
      )
        n.flow = "xtls-rprx-vision";
      return n;
    });
  }
  async function realityKeys() {
    if (!form.node) return;
    setBusy(true);
    try {
      const k: any = await api(`/nodes/${form.node}/reality-keypair/`, {
        method: "POST",
      });
      setForm((f) => ({
        ...f,
        reality_private_key: k.private_key,
        reality_public_key: k.public_key,
      }));
      setMsg(t("REALITY key pair generated."));
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function wgKeys() {
    if (!form.node) return;
    setBusy(true);
    try {
      const k: any = await api(`/nodes/${form.node}/wireguard-keypair/`, {
        method: "POST",
      });
      setForm((f) => ({
        ...f,
        wg_secret_key: k.private_key,
        wg_public_key: k.public_key,
      }));
      setMsg(t("WireGuard server key pair generated."));
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  function payload() {
    const p: any = {
      core: form.core,
      node: Number(form.node),
      name: form.name,
      listen: form.listen,
      port: Number(form.port),
      protocol: form.protocol,
      transport: form.transport,
      security: form.security,
      flow: form.flow,
      path: form.path,
      host_header: form.host_header,
      service_name: form.service_name,
      tls_server_name: form.tls_server_name,
      tls_cert_file: form.tls_cert_file,
      tls_key_file: form.tls_key_file,
      reality_dest: form.reality_dest,
      reality_server_name: form.reality_server_name,
      reality_private_key: form.reality_private_key,
      reality_public_key: form.reality_public_key,
      reality_short_id: form.reality_short_id,
      enabled: editing?.enabled ?? true,
      protocol_settings: { ...(editing?.protocol_settings || {}) },
      stream_settings: JSON.parse(form.stream_json || "{}"),
    };
    delete p.protocol_settings.fallbacks;
    if (
      form.core === "xray" &&
      ["vless", "trojan"].includes(form.protocol) &&
      form.transport === "raw" &&
      form.security !== "reality" &&
      form.fallbacks.length
    )
      p.protocol_settings.fallbacks = form.fallbacks;
    if (["wireguard", "amneziawg"].includes(form.protocol))
      p.protocol_settings = {
        secret_key: form.wg_secret_key,
        public_key: form.wg_public_key,
        mtu: Number(form.wg_mtu || 1420),
        dns: form.wg_dns || "1.1.1.1",
      };
    if (form.protocol === "amneziawg")
      Object.entries(form.awg).forEach(([key, value]) => {
        p.protocol_settings[key] = key.startsWith("h")
          ? value.trim()
          : Number(value);
      });
    if (form.protocol === "mtproto")
      p.protocol_settings = {
        fake_tls_host: form.mtproto_host.trim().toLowerCase(),
      };
    if (form.protocol === "socks")
      p.protocol_settings = {
        udp: form.socks_udp,
        udp_ip:
          nodes.find((n) => String(n.id) === form.node)?.public_host || "",
      };
    if (form.protocol === "shadowsocks")
      p.protocol_settings = { method: form.ss_method };
    if (form.protocol === "shadowtls")
      p.protocol_settings = {
        handshake_server: form.st_handshake.trim(),
        handshake_port: Number(form.st_port || 443),
        psk: form.st_psk.trim(),
      };
    if (form.protocol === "hysteria")
      p.protocol_settings = {
        udp_idle_timeout: Number(form.hy_udp_idle || 60),
        masquerade_url: form.hy_masquerade,
      };
    if (form.protocol === "tunnel")
      p.protocol_settings = {
        allowed_network: form.tunnel_network,
        rewrite_address: form.tunnel_address || "localhost",
        rewrite_port: Number(form.tunnel_port || 0),
        follow_redirect: form.tunnel_redirect,
      };
    if (form.protocol === "tun")
      p.protocol_settings = {
        name: form.tun_name || "veloray-tun0",
        mtu: Number(form.tun_mtu || 1500),
        gateway: form.tun_gateway,
        dns: form.tun_dns,
        auto_routes: form.tun_routes,
        outbound_interface: form.tun_interface || "auto",
      };
    if (p.security !== "reality")
      for (const k of [
        "reality_dest",
        "reality_server_name",
        "reality_private_key",
        "reality_public_key",
        "reality_short_id",
      ])
        p[k] = "";
    if (p.security !== "tls")
      for (const k of ["tls_server_name", "tls_cert_file", "tls_key_file"])
        p[k] = "";
    return p;
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setMsg("");
    try {
      if (editing) {
        await api(`/inbounds/${editing.id}/`, {
          method: "PATCH",
          body: JSON.stringify(payload()),
        });
        setMsg(t("Inbound updated, validated and deployed."));
      } else {
        await api("/inbounds/", {
          method: "POST",
          body: JSON.stringify(payload()),
        });
        setMsg(t("Inbound created, validated and deployed."));
      }
      const keep = form.node;
      setEditorOpen(false);
      setEditing(null);
      setForm(fresh(keep));
      await load();
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  function beginEdit(i: InboundItem) {
    const ps = i.protocol_settings || {};
    setEditorOpen(true);
    setEditing(i);
    setForm({
      fallbacks: structuredClone(
        Array.isArray(ps.fallbacks) ? (ps.fallbacks as FallbackEntry[]) : [],
      ),
      core: i.core || "xray",
      node: String(i.node),
      name: i.name,
      listen: i.listen || "0.0.0.0",
      port: String(i.port),
      protocol: i.protocol,
      transport: i.transport,
      security: i.security,
      flow: i.flow || "",
      path: i.path || "/",
      host_header: i.host_header || "",
      service_name: i.service_name || "",
      tls_server_name: i.tls_server_name || "",
      tls_cert_file: i.tls_cert_file || "",
      tls_key_file: i.tls_key_file || "",
      reality_dest: i.reality_dest || "",
      reality_server_name: i.reality_server_name || "",
      reality_private_key: i.reality_private_key || "",
      reality_public_key: i.reality_public_key || "",
      reality_short_id: i.reality_short_id || "",
      wg_secret_key: String(ps.secret_key || ""),
      wg_public_key: String(ps.public_key || ""),
      wg_mtu: String(ps.mtu || 1420),
      wg_dns: String(ps.dns || "1.1.1.1"),
      socks_udp: ps.udp !== false,
      ss_method: String(ps.method || "aes-256-gcm"),
      awg: Object.fromEntries(
        Object.entries(fresh().awg).map(([key, value]) => [
          key,
          String(ps[key] ?? value),
        ]),
      ),
      mtproto_host: String(ps.fake_tls_host || "www.cloudflare.com"),
      st_handshake: String(ps.handshake_server || ""),
      st_port: String(ps.handshake_port || 443),
      st_psk: String(ps.psk || ""),
      hy_udp_idle: String(ps.udp_idle_timeout || 60),
      hy_masquerade: String(ps.masquerade_url || ""),
      tunnel_network: String(ps.allowed_network || "tcp"),
      tunnel_address: String(ps.rewrite_address || "localhost"),
      tunnel_port: String(ps.rewrite_port || 0),
      tunnel_redirect: Boolean(ps.follow_redirect),
      tun_name: String(ps.name || "veloray-tun0"),
      tun_mtu: String(ps.mtu || 1500),
      tun_gateway: Array.isArray(ps.gateway)
        ? ps.gateway.join(", ")
        : String(ps.gateway || "10.66.0.1/16"),
      tun_dns: Array.isArray(ps.dns)
        ? ps.dns.join(", ")
        : String(ps.dns || "1.1.1.1,8.8.8.8"),
      tun_routes: Array.isArray(ps.auto_routes)
        ? ps.auto_routes.join(", ")
        : String(ps.auto_routes || ""),
      tun_interface: String(ps.outbound_interface || "auto"),
      stream_json: JSON.stringify(i.stream_settings || {}, null, 2),
    });
    window.scrollTo({ top: 0, behavior: "smooth" });
  }
  function cancelEdit() {
    const node = form.node;
    setEditorOpen(false);
    setEditing(null);
    setForm(fresh(node));
  }
  async function deploy(i: InboundItem) {
    try {
      setMsg("Deploying…");
      await api(`/nodes/${i.node}/deploy/`, { method: "POST" });
      setMsg(tx("{0} validated and deployed.", i.node_name));
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function toggle(i: InboundItem) {
    try {
      await api(`/inbounds/${i.id}/`, {
        method: "PATCH",
        body: JSON.stringify({ enabled: !i.enabled }),
      });
      await load();
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function remove(i: InboundItem) {
    if (!confirm(tx("Delete inbound {0}?", i.name))) return;
    try {
      await api(`/inbounds/${i.id}/`, { method: "DELETE" });
      await load();
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  function beginClone(i: InboundItem) {
    setCloning(i);
    setCloneNode(String(i.node));
    setClonePort(
      i.protocol === "tun" ? "0" : String(Math.min(65535, i.port + 1)),
    );
    setCloneName(tx("{0} copy", i.name));
  }
  async function clone() {
    if (!cloning || cloneBusy) return;
    setCloneBusy(true);
    try {
      await api(`/inbounds/${cloning.id}/clone/`, {
        method: "POST",
        body: JSON.stringify({
          node: Number(cloneNode),
          port: Number(clonePort),
          name: cloneName,
        }),
      });
      setCloning(null);
      await load();
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setCloneBusy(false);
    }
  }
  const [editorOpen, setEditorOpen] = useState(false);
  const reality = form.security === "reality",
    tls = form.security === "tls",
    vision = form.protocol === "vless" && form.transport === "raw" && reality;
  const plaintext = form.protocol === "http" || form.protocol === "socks";
  const protocolCounts = useMemo(
    () =>
      Object.fromEntries(
        protocolOptions.map(([p]) => [
          p,
          items.filter((i) => i.protocol === p).length,
        ]),
      ),
    [items],
  );
  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Inbounds")}
        description=""
        actions={
          <Button
            onClick={() => {
              cancelEdit();
              setEditorOpen(true);
            }}
          >
            <Plus size={15} />
            {t("New inbound")}
          </Button>
        }
      />
      {msg && (
        <div
          role="status"
          aria-live="polite"
          className="flex items-start gap-2 rounded-xl border border-[var(--border)] bg-[var(--card)] px-4 py-3 text-[11px] text-[var(--muted-strong)]"
        >
          <FaIcon icon="circle-info" className="mt-0.5 text-[var(--info)]" />
          {msg}
        </div>
      )}
      <div className="space-y-4">
        {editorOpen && (
          <Dialog
            label={editing ? t("Edit inbound") : t("New inbound")}
            onClose={cancelEdit}
            padded={false}
            size="lg"
          >
            <Card>
              <CardHeader>
                <div>
                  <CardTitle>
                    {editing ? t("Edit inbound") : t("New inbound")}
                  </CardTitle>
                </div>
                {true && (
                  <Button variant="ghost" size="icon" onClick={cancelEdit}>
                    <X className="h-4 w-4" />
                  </Button>
                )}
              </CardHeader>
              <CardContent>
                <form onSubmit={save} className="space-y-5">
                  <Field label="Core">
                    <Select
                      className="input"
                      value={form.core}
                      onChange={(e) => {
                        set("core", e.target.value);
                        set(
                          "protocol",
                          e.target.value === "native" ? "mtproto" : "vless",
                        );
                        set("transport", "raw");
                        set("security", "none");
                        set("flow", "");
                      }}
                    >
                      <option value="xray">Xray · Primary</option>
                      <option value="singbox">sing-box</option>
                      <option value="native">
                        VeloRay · MTProto / AmneziaWG
                      </option>
                    </Select>
                  </Field>
                  {!editing && form.core !== "native" && (
                    <label className="label">
                      {t("Connection preset")}
                      <Select
                        className="input"
                        aria-label={t("Connection preset")}
                        value=""
                        onChange={(e) => preset(e.target.value)}
                        disabled={busy}
                      >
                        <option value="">{t("Choose a starting point")}</option>
                        <option value="reality">
                          {t("VLESS · REALITY · Vision")}
                        </option>
                        <option value="websocket">
                          {t("VLESS · WebSocket · TLS")}
                        </option>
                        <option value="trojan">{t("Trojan · TLS")}</option>
                        {form.core === "xray" && (
                          <option value="wireguard">
                            {t("WireGuard · UDP")}
                          </option>
                        )}
                        <option value="hysteria">
                          {t("Hysteria2 · UDP · TLS")}
                        </option>
                      </Select>
                    </label>
                  )}
                  <section>
                    <div className="panel-section-title mb-3">
                      {t("Listener")}
                    </div>
                    <div className="grid gap-3 sm:grid-cols-2">
                      <Field label={t("Node")} className="sm:col-span-2">
                        <Select
                          className="input"
                          value={form.node}
                          onChange={(e) => set("node", e.target.value)}
                          required
                        >
                          <option value="">{t("Select node")}</option>
                          {nodes.map((n) => (
                            <option key={n.id} value={n.id}>
                              {n.is_local ? "★ " : ""}
                              {n.name}
                            </option>
                          ))}
                        </Select>
                      </Field>
                      <Field label={t("Name")}>
                        <Input
                          className="input"
                          value={form.name}
                          onChange={(e) => set("name", e.target.value)}
                          required
                        />
                      </Field>
                      <Field
                        label={
                          form.protocol === "tun"
                            ? t("Port (not used)")
                            : t("Port")
                        }
                      >
                        <Input
                          className="input"
                          type="number"
                          min={form.protocol === "tun" ? 0 : 1}
                          max="65535"
                          value={form.port}
                          onChange={(e) => set("port", e.target.value)}
                          disabled={form.protocol === "tun"}
                          required
                        />
                      </Field>
                      {form.protocol !== "tun" && (
                        <Field
                          label={t("Listen address")}
                          className="sm:col-span-2"
                        >
                          <Input
                            className="input"
                            value={form.listen}
                            onChange={(e) => set("listen", e.target.value)}
                            placeholder={t("0.0.0.0 or ::")}
                            required
                          />
                          <span className="mt-1 text-[10px] font-normal text-[var(--muted)]">
                            {t(
                              "Use 0.0.0.0 for IPv4 or :: for IPv6. Enter a specific IP to restrict the listener.",
                            )}
                          </span>
                        </Field>
                      )}
                    </div>
                    {form.protocol !== "tun" && (
                      <div className="mt-3 space-y-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={checkPort}
                          disabled={portBusy || busy || !form.node}
                        >
                          <FaIcon icon="shield-halved" />
                          {portBusy ? t("Checking port…") : t("Check port")}
                        </Button>
                        {portResult && portSignature === signature && (
                          <p
                            role="status"
                            className="break-words text-[11px] leading-5 text-[var(--muted-strong)]"
                          >
                            {portResult}
                          </p>
                        )}
                      </div>
                    )}
                  </section>
                  <section>
                    <div className="panel-section-title mb-3">
                      {t("Protocol")}
                    </div>
                    <Select
                      className="input"
                      aria-label={t("Protocol")}
                      value={form.protocol}
                      onChange={(e) => set("protocol", e.target.value)}
                    >
                      {protocolOptions
                        .filter(
                          ([v]) =>
                            (form.core === "xray" &&
                              ![
                                "tuic",
                                "naive",
                                "snell",
                                "shadowtls",
                                "mtproto",
                                "amneziawg",
                              ].includes(v)) ||
                            (form.core === "singbox" &&
                              [
                                "vless",
                                "vmess",
                                "trojan",
                                "shadowsocks",
                                "hysteria",
                                "tuic",
                                "naive",
                                "snell",
                                "shadowtls",
                                "http",
                                "socks",
                              ].includes(v)) ||
                            (form.core === "native" &&
                              ["mtproto", "amneziawg"].includes(v)),
                        )
                        .map(([v, l]) => (
                          <option key={v} value={v}>
                            {t(l)}
                          </option>
                        ))}
                    </Select>
                    {plaintext && (
                      <div className="mt-3 rounded-lg border border-amber-500/20 bg-amber-500/7 px-3 py-2 text-[10px] leading-5 text-amber-500">
                        {t(
                          "HTTP and SOCKS are plaintext proxy protocols. Prefer local/LAN use or place them behind a protected transport.",
                        )}
                      </div>
                    )}
                  </section>
                  {form.protocol === "mtproto" && (
                    <section className="space-y-3">
                      <Field label="FakeTLS hostname">
                        <Input
                          value={form.mtproto_host}
                          onChange={(e) => set("mtproto_host", e.target.value)}
                          placeholder="www.cloudflare.com"
                          autoComplete="off"
                          required
                        />
                      </Field>
                      <p className="text-xs leading-6 text-[var(--muted-strong)]">
                        {t(
                          "Use a public HTTPS hostname reachable from this node. Each user receives a separate Telegram secret, traffic counter and revocable connection. No local TLS certificate is needed.",
                        )}
                      </p>
                    </section>
                  )}
                  {![
                    "wireguard",
                    "tunnel",
                    "tun",
                    "mtproto",
                    "amneziawg",
                  ].includes(form.protocol) && (
                    <section>
                      <div className="panel-section-title mb-3">
                        {t("Transport")}
                      </div>
                      {form.protocol === "hysteria" ? (
                        <div className="grid grid-cols-2">
                          <Choice
                            value="hysteria"
                            label={t("Hysteria2 / QUIC")}
                            active
                            onClick={() => {}}
                          />
                        </div>
                      ) : (
                        <Select
                          className="input"
                          aria-label={t("Transport")}
                          value={form.transport}
                          onChange={(e) => set("transport", e.target.value)}
                        >
                          {transportOptions
                            .filter(
                              ([v]) =>
                                form.core === "xray" ||
                                ["raw", "ws", "grpc", "httpupgrade"].includes(
                                  v,
                                ),
                            )
                            .map(([v, l]) => (
                              <option
                                key={v}
                                value={v}
                                disabled={
                                  [
                                    "shadowsocks",
                                    "socks",
                                    "tuic",
                                    "naive",
                                    "snell",
                                    "shadowtls",
                                  ].includes(form.protocol) && v !== "raw"
                                }
                              >
                                {t(l)}
                              </option>
                            ))}
                        </Select>
                      )}
                      {["ws", "xhttp", "httpupgrade"].includes(
                        form.transport,
                      ) && (
                        <div className="mt-3 grid gap-3 sm:grid-cols-2">
                          <Field label={t("Path")}>
                            <Input
                              className="input"
                              value={form.path}
                              onChange={(e) => set("path", e.target.value)}
                              placeholder={t("/veloray")}
                            />
                          </Field>
                          {["ws", "httpupgrade"].includes(form.transport) && (
                            <Field label={t("Host header")}>
                              <Input
                                className="input"
                                value={form.host_header}
                                onChange={(e) =>
                                  set("host_header", e.target.value)
                                }
                                placeholder={t("cdn.example.com")}
                              />
                            </Field>
                          )}
                        </div>
                      )}
                      {form.transport === "grpc" && (
                        <div className="mt-3">
                          <Field label={t("gRPC service")}>
                            <Input
                              className="input"
                              value={form.service_name}
                              onChange={(e) =>
                                set("service_name", e.target.value)
                              }
                              placeholder={t("veloray-grpc")}
                            />
                          </Field>
                        </div>
                      )}
                    </section>
                  )}
                  {![
                    "wireguard",
                    "tunnel",
                    "tun",
                    "mtproto",
                    "amneziawg",
                  ].includes(form.protocol) && (
                    <section>
                      <div className="panel-section-title mb-3">
                        {t("Security")}
                      </div>
                      <div className="grid grid-cols-3 gap-2">
                        {[
                          ["none", t("None")],
                          ["tls", t("TLS")],
                          ["reality", t("REALITY")],
                        ].map(([v, l]) => (
                          <Choice
                            key={v}
                            value={v}
                            label={l}
                            active={form.security === v}
                            disabled={
                              (["hysteria", "tuic", "naive"].includes(
                                form.protocol,
                              ) &&
                                v !== "tls") ||
                              ((form.protocol === "shadowsocks" ||
                                form.protocol === "socks" ||
                                ["snell", "shadowtls"].includes(
                                  form.protocol,
                                )) &&
                                v !== "none") ||
                              (v === "reality" &&
                                (form.protocol !== "vless" ||
                                  !["raw", "grpc", "xhttp"].includes(
                                    form.transport,
                                  )))
                            }
                            onClick={(v) => set("security", v)}
                          />
                        ))}
                      </div>
                      {vision && (
                        <div className="mt-3">
                          <Field label={t("VLESS flow")}>
                            <Select
                              className="input"
                              value={form.flow}
                              onChange={(e) => set("flow", e.target.value)}
                            >
                              <option value="xtls-rprx-vision">
                                {t("XTLS Vision")}
                              </option>
                              <option value="">{t("None")}</option>
                            </Select>
                          </Field>
                        </div>
                      )}
                      {tls && (
                        <div className="mt-3 grid gap-3 sm:grid-cols-2">
                          <Field label={t("TLS certificate")}>
                            <Input
                              className="input"
                              value={form.tls_cert_file}
                              onChange={(e) =>
                                set("tls_cert_file", e.target.value)
                              }
                              placeholder={t(
                                "/etc/letsencrypt/live/.../fullchain.pem",
                              )}
                            />
                          </Field>
                          <Field label={t("TLS private key")}>
                            <Input
                              className="input"
                              value={form.tls_key_file}
                              onChange={(e) =>
                                set("tls_key_file", e.target.value)
                              }
                              placeholder={t(
                                "/etc/letsencrypt/live/.../privkey.pem",
                              )}
                            />
                          </Field>
                          <Field
                            label={t("Server name")}
                            className="sm:col-span-2"
                          >
                            <SNIScanner
                              node={form.node}
                              value={form.tls_server_name}
                              onChange={(v) => {
                                set("tls_server_name", v);
                              }}
                            />
                          </Field>
                        </div>
                      )}
                      {reality && (
                        <div className="mt-3 space-y-3">
                          <div className="flex items-center justify-between rounded-lg border border-[var(--border)] bg-[var(--surface-2)] px-3 py-2">
                            <div>
                              <div className="text-[11px] font-semibold">
                                {t("REALITY identity")}
                              </div>
                              <div className="mt-0.5 text-[9px] text-[var(--muted)]">
                                {t(
                                  "Generate X25519 keys on the selected node.",
                                )}
                              </div>
                            </div>
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={realityKeys}
                              disabled={busy}
                            >
                              <KeyRound className="h-3.5 w-3.5" />
                              {t("Generate")}
                            </Button>
                          </div>
                          <div className="grid gap-3 sm:grid-cols-2">
                            <Field label={t("Destination")}>
                              <Input
                                className="input"
                                value={form.reality_dest}
                                onChange={(e) =>
                                  set("reality_dest", e.target.value)
                                }
                              />
                            </Field>
                            <Field label={t("Server name")}>
                              <SNIScanner
                                node={form.node}
                                value={form.reality_server_name}
                                onChange={(v) => {
                                  set("reality_server_name", v);
                                  set("reality_dest", v + ":443");
                                }}
                              />
                            </Field>
                            <Field label={t("Private key")}>
                              <Input
                                className="input mono"
                                value={form.reality_private_key}
                                onChange={(e) =>
                                  set("reality_private_key", e.target.value)
                                }
                              />
                            </Field>
                            <Field label={t("Public key")}>
                              <Input
                                className="input mono"
                                value={form.reality_public_key}
                                onChange={(e) =>
                                  set("reality_public_key", e.target.value)
                                }
                              />
                            </Field>
                          </div>
                        </div>
                      )}
                    </section>
                  )}
                  {["wireguard", "amneziawg"].includes(form.protocol) && (
                    <section>
                      <div className="panel-section-title mb-3">
                        {form.protocol === "amneziawg"
                          ? "AmneziaWG"
                          : "WireGuard"}
                      </div>
                      <div className="flex items-center justify-between rounded-lg border border-[var(--border)] bg-[var(--surface-2)] px-3 py-2">
                        <div>
                          <div className="text-[11px] font-semibold">
                            {t("Server key pair")}
                          </div>
                          <div className="mt-0.5 text-[9px] text-[var(--muted)]">
                            {t("Generated by Xray on the selected node.")}
                          </div>
                        </div>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={wgKeys}
                          disabled={busy}
                        >
                          <KeyRound className="h-3.5 w-3.5" />
                          {t("Generate")}
                        </Button>
                      </div>
                      <div className="mt-3 grid gap-3 sm:grid-cols-2">
                        <Field label={t("Private key")}>
                          <Input
                            className="input mono"
                            value={form.wg_secret_key}
                            onChange={(e) =>
                              set("wg_secret_key", e.target.value)
                            }
                          />
                        </Field>
                        <Field label={t("Public key")}>
                          <Input
                            className="input mono"
                            value={form.wg_public_key}
                            onChange={(e) =>
                              set("wg_public_key", e.target.value)
                            }
                          />
                        </Field>
                        <Field label={t("MTU")}>
                          <Input
                            className="input"
                            type="number"
                            min={
                              form.protocol === "amneziawg" ? "1280" : "1200"
                            }
                            max={
                              form.protocol === "amneziawg" ? "1420" : "9000"
                            }
                            value={form.wg_mtu}
                            onChange={(e) => set("wg_mtu", e.target.value)}
                          />
                        </Field>
                        <Field label={t("Client DNS")}>
                          <Input
                            className="input"
                            value={form.wg_dns}
                            onChange={(e) => set("wg_dns", e.target.value)}
                          />
                        </Field>
                      </div>
                    </section>
                  )}
                  {form.protocol === "amneziawg" && (
                    <section className="space-y-3">
                      <div className="panel-section-title">
                        {t("AmneziaWG obfuscation")}
                      </div>
                      <p className="text-xs leading-6 text-[var(--muted-strong)]">
                        {t(
                          "These values must match the client configuration. Exports include them automatically. UDP must be allowed on the selected port.",
                        )}
                      </p>
                      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                        {Object.entries(form.awg).map(([key, value]) => (
                          <Field
                            key={key}
                            label={
                              key === "jc"
                                ? "Jc · Junk count"
                                : key === "jmin"
                                  ? "Jmin · Minimum size"
                                  : key === "jmax"
                                    ? "Jmax · Maximum size"
                                    : key.toUpperCase()
                            }
                          >
                            <Input
                              type="number"
                              value={value}
                              min={key.startsWith("h") ? 5 : 0}
                              max={
                                key.startsWith("h")
                                  ? 4294967295
                                  : key === "jc"
                                    ? 128
                                    : key === "s4"
                                      ? 64
                                      : 1280
                              }
                              onChange={(e) =>
                                set("awg", {
                                  ...form.awg,
                                  [key]: e.target.value,
                                })
                              }
                              required
                            />
                          </Field>
                        ))}
                      </div>
                    </section>
                  )}
                  {form.protocol === "socks" && (
                    <section>
                      <div className="panel-section-title mb-3">
                        {t("SOCKS options")}
                      </div>
                      <label className="flex items-center justify-between rounded-lg border border-[var(--border)] bg-[var(--surface-2)] px-3 py-2.5 text-[11px]">
                        <span>{t("Enable UDP relay")}</span>
                        <Input
                          type="checkbox"
                          checked={form.socks_udp}
                          onChange={(e) => set("socks_udp", e.target.checked)}
                        />
                      </label>
                    </section>
                  )}
                  {form.protocol === "shadowsocks" && (
                    <section>
                      <div className="panel-section-title mb-3">
                        {t("Shadowsocks")}
                      </div>
                      <Field label={t("Cipher")}>
                        <Select
                          className="input"
                          value={form.ss_method}
                          onChange={(e) => set("ss_method", e.target.value)}
                        >
                          <option value="aes-256-gcm">
                            {t("AES-256-GCM")}
                          </option>
                          <option value="chacha20-poly1305">
                            {t("ChaCha20-Poly1305")}
                          </option>
                        </Select>
                      </Field>
                    </section>
                  )}
                  {form.protocol === "shadowtls" && (
                    <section>
                      <div className="panel-section-title mb-3">
                        ShadowTLS v3
                      </div>
                      <p className="mb-3 text-xs text-[var(--muted)]">
                        {t(
                          "TLS 1.3 handshake with authenticated Shadowsocks 2022 inside. TCP only; download sing-box JSON or Mihomo YAML from the subscription page.",
                        )}
                      </p>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <Field label="Handshake server">
                          <Input
                            className="input"
                            required
                            value={form.st_handshake}
                            onChange={(e) =>
                              set("st_handshake", e.target.value)
                            }
                            placeholder="tls.example.com"
                          />
                        </Field>
                        <Field label="Handshake port">
                          <Input
                            className="input"
                            type="number"
                            required
                            min="1"
                            max="65535"
                            value={form.st_port}
                            onChange={(e) => set("st_port", e.target.value)}
                          />
                        </Field>
                        <Field label="Inner server key">
                          <Input
                            className="input"
                            type="password"
                            autoComplete="new-password"
                            value={form.st_psk}
                            onChange={(e) => set("st_psk", e.target.value)}
                            placeholder={t(
                              "Generated automatically on creation",
                            )}
                          />
                        </Field>
                      </div>
                      <p className="mt-2 text-xs text-[var(--muted)]">
                        {t(
                          "Use a hostname with a valid TLS 1.3 certificate. Each client gets separate inner and ShadowTLS credentials automatically.",
                        )}
                      </p>
                    </section>
                  )}
                  {form.protocol === "hysteria" && (
                    <section>
                      <div className="panel-section-title mb-3">
                        {t("Hysteria2")}
                      </div>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <Field label={t("UDP idle timeout")}>
                          <Input
                            className="input"
                            type="number"
                            min="10"
                            value={form.hy_udp_idle}
                            onChange={(e) => set("hy_udp_idle", e.target.value)}
                          />
                        </Field>
                        <Field label={t("Masquerade URL")}>
                          <Input
                            className="input"
                            value={form.hy_masquerade}
                            onChange={(e) =>
                              set("hy_masquerade", e.target.value)
                            }
                            placeholder={t("https://example.com")}
                          />
                        </Field>
                      </div>
                    </section>
                  )}
                  {form.protocol === "tunnel" && (
                    <section>
                      <div className="panel-section-title mb-3">
                        {t("Tunnel / port forwarding")}
                      </div>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <Field label={t("Allowed network")}>
                          <Select
                            className="input"
                            value={form.tunnel_network}
                            onChange={(e) =>
                              set("tunnel_network", e.target.value)
                            }
                          >
                            <option value="tcp">{t("TCP")}</option>
                            <option value="udp">{t("UDP")}</option>
                            <option value="tcp,udp">{t("TCP + UDP")}</option>
                          </Select>
                        </Field>
                        <Field label={t("Rewrite address")}>
                          <Input
                            className="input"
                            value={form.tunnel_address}
                            onChange={(e) =>
                              set("tunnel_address", e.target.value)
                            }
                            placeholder={t("target.example.com")}
                          />
                        </Field>
                        <Field label={t("Rewrite port")}>
                          <Input
                            className="input"
                            type="number"
                            min="0"
                            max="65535"
                            value={form.tunnel_port}
                            onChange={(e) => set("tunnel_port", e.target.value)}
                          />
                        </Field>
                        <label className="flex items-center justify-between rounded-lg border border-[var(--border)] bg-[var(--surface-2)] px-3 py-2.5 text-[11px]">
                          <span>{t("Follow redirected destination")}</span>
                          <Input
                            type="checkbox"
                            checked={form.tunnel_redirect}
                            onChange={(e) =>
                              set("tunnel_redirect", e.target.checked)
                            }
                          />
                        </label>
                      </div>
                    </section>
                  )}
                  {form.protocol === "tun" && (
                    <section>
                      <div className="panel-section-title mb-3">
                        {t("TUN interface")}
                      </div>
                      <div className="rounded-lg border border-amber-500/20 bg-amber-500/7 px-3 py-2 text-[10px] leading-5 text-amber-500">
                        {t(
                          "TUN changes host routing. VeloRay validates the Xray config, but route design should still be tested carefully to avoid loops.",
                        )}
                      </div>
                      <div className="mt-3 grid gap-3 sm:grid-cols-2">
                        <Field label={t("Interface name")}>
                          <Input
                            className="input"
                            value={form.tun_name}
                            onChange={(e) => set("tun_name", e.target.value)}
                          />
                        </Field>
                        <Field label={t("MTU")}>
                          <Input
                            className="input"
                            type="number"
                            min="576"
                            max="9000"
                            value={form.tun_mtu}
                            onChange={(e) => set("tun_mtu", e.target.value)}
                          />
                        </Field>
                        <Field label={t("Gateway CIDRs")}>
                          <Input
                            className="input"
                            value={form.tun_gateway}
                            onChange={(e) => set("tun_gateway", e.target.value)}
                            placeholder="10.66.0.1/16"
                          />
                        </Field>
                        <Field label={t("DNS servers")}>
                          <Input
                            className="input"
                            value={form.tun_dns}
                            onChange={(e) => set("tun_dns", e.target.value)}
                            placeholder="1.1.1.1, 8.8.8.8"
                          />
                        </Field>
                        <Field
                          label={t("Automatic route CIDRs")}
                          className="sm:col-span-2"
                        >
                          <Input
                            className="input"
                            value={form.tun_routes}
                            onChange={(e) => set("tun_routes", e.target.value)}
                            placeholder={t("0.0.0.0/0, ::/0 (optional)")}
                          />
                        </Field>
                        <Field
                          label={t("Outbound interface")}
                          className="sm:col-span-2"
                        >
                          <Input
                            className="input"
                            value={form.tun_interface}
                            onChange={(e) =>
                              set("tun_interface", e.target.value)
                            }
                            placeholder={t("auto")}
                          />
                        </Field>
                      </div>
                    </section>
                  )}
                  {form.core === "xray" &&
                    ["vless", "trojan"].includes(form.protocol) &&
                    form.transport === "raw" &&
                    form.security !== "reality" && (
                      <InboundFallbacks
                        value={form.fallbacks}
                        onChange={(value) => set("fallbacks", value)}
                      />
                    )}
                  <details className="rounded-lg border border-[var(--border)] p-3">
                    <summary className="cursor-pointer text-[12px] font-medium">
                      {t("Advanced stream settings")}
                    </summary>
                    <label className="label mt-3">
                      {t("Stream settings JSON")}
                      <Textarea
                        className="input min-h-40 font-mono text-[11px]"
                        value={form.stream_json}
                        onChange={(e) => set("stream_json", e.target.value)}
                        spellCheck={false}
                      />
                    </label>
                  </details>
                  <Button
                    type="submit"
                    className="w-full"
                    disabled={busy || !form.node}
                  >
                    <Plus className="h-3.5 w-3.5" />
                    {busy
                      ? "Saving…"
                      : editing
                        ? t("Save & deploy")
                        : t("Create & deploy")}
                  </Button>
                </form>
              </CardContent>
            </Card>
          </Dialog>
        )}
        <Card>
          <CardHeader>
            <div>
              <CardTitle>{t("Configured inbounds")}</CardTitle>
            </div>
            <Badge tone="blue">Xray + sing-box</Badge>
          </CardHeader>
          <CardContent className="p-0">
            <div className="hidden md:block table-wrap">
              <table className="data-table min-w-[940px]">
                <thead>
                  <tr>
                    <th>{t("Name")}</th>
                    <th>{t("Node")}</th>
                    <th>{t("Protocol")}</th>
                    <th>{t("Transport")}</th>
                    <th>{t("Security")}</th>
                    <th>{t("Port")}</th>
                    <th>{t("Clients")}</th>
                    <th>{t("Status")}</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((i) => (
                    <tr key={i.id}>
                      <td className="font-semibold text-[var(--fg)]">
                        {i.name}
                      </td>
                      <td>{i.node_name}</td>
                      <td>
                        <Badge tone="blue">
                          {i.protocol === "hysteria"
                            ? "Hysteria2"
                            : i.protocol.toUpperCase()}
                        </Badge>
                      </td>
                      <td>
                        {[
                          "wireguard",
                          "tunnel",
                          "tun",
                          "amneziawg",
                          "mtproto",
                        ].includes(i.protocol)
                          ? t("Native")
                          : i.transport}
                      </td>
                      <td>
                        {["wireguard", "amneziawg"].includes(i.protocol)
                          ? t("WireGuard")
                          : ["tunnel", "tun"].includes(i.protocol)
                            ? t("Native")
                            : i.security}
                      </td>
                      <td className="mono">
                        {i.protocol === "tun" ? "—" : i.port}
                      </td>
                      <td>{i.client_count}</td>
                      <td>
                        <Badge tone={i.enabled ? "green" : "neutral"}>
                          {i.enabled ? t("Enabled") : t("Paused")}
                        </Badge>
                      </td>
                      <td>
                        <div className="flex justify-end gap-1">
                          <Button
                            title={t("Clone")}
                            size="icon"
                            variant="ghost"
                            onClick={() => beginClone(i)}
                          >
                            <Copy className="h-3.5 w-3.5" />
                          </Button>
                          <Button
                            title={i.enabled ? t("Pause") : t("Enable")}
                            size="icon"
                            variant="ghost"
                            onClick={() => toggle(i)}
                          >
                            {i.enabled ? (
                              <Pause className="h-3.5 w-3.5" />
                            ) : (
                              <Play className="h-3.5 w-3.5" />
                            )}
                          </Button>
                          <Button
                            title={t("Edit")}
                            size="icon"
                            variant="ghost"
                            onClick={() => beginEdit(i)}
                          >
                            <Pencil className="h-3.5 w-3.5" />
                          </Button>
                          <Button
                            title={t("Deploy")}
                            size="icon"
                            variant="outline"
                            onClick={() => deploy(i)}
                          >
                            <Rocket className="h-3.5 w-3.5" />
                          </Button>
                          <Button
                            title={t("Delete")}
                            size="icon"
                            variant="ghost"
                            onClick={() => remove(i)}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!items.length && (
              <div className="p-12 text-center text-[11px] text-[var(--muted)]">
                {t("Create your first listener on Local Node.")}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
      {cloning && (
        <Dialog
          label={t("Clone inbound")}
          onClose={() => setCloning(null)}
          padded={false}
        >
          <div className="flex items-center justify-between border-b border-[var(--border)] px-5 py-4">
            <div>
              <div className="text-[14px] font-semibold">
                {t("Clone inbound")}
              </div>
              <div className="mt-1 text-[10px] text-[var(--muted)]">
                {t("Duplicate")} {cloning.name} {t("to another node or port.")}
              </div>
            </div>
            <Button
              size="icon"
              variant="ghost"
              onClick={() => setCloning(null)}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
          <div className="grid gap-3 p-5">
            <Field label={t("Name")}>
              <Input
                className="input"
                value={cloneName}
                onChange={(e) => setCloneName(e.target.value)}
              />
            </Field>
            <Field label={t("Node")}>
              <Select
                className="input"
                value={cloneNode}
                onChange={(e) => setCloneNode(e.target.value)}
              >
                {nodes.map((n) => (
                  <option key={n.id} value={n.id}>
                    {n.name}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label={t("Port")}>
              <Input
                className="input"
                type="number"
                value={clonePort}
                onChange={(e) => setClonePort(e.target.value)}
                disabled={cloning.protocol === "tun"}
              />
            </Field>
          </div>
          <div className="flex justify-end gap-2 border-t border-[var(--border)] px-5 py-4">
            <Button variant="outline" onClick={() => setCloning(null)}>
              {t("Cancel")}
            </Button>
            <Button onClick={clone} disabled={cloneBusy}>
              {cloneBusy ? "Cloning…" : t("Clone & deploy")}
            </Button>
          </div>
        </Dialog>
      )}
    </div>
  );
}
