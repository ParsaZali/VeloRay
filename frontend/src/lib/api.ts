import { t, localeName } from "./i18n";
export const API_BASE = import.meta.env.VITE_API_URL || "/api";

export type PortReport = {
  ports: {
    tag: string;
    listen: string;
    port: number;
    network: string;
    state: "free" | "xray" | "conflict";
    owner?: string;
  }[];
  error?: string;
};

function cookie(name: string) {
  return (
    document.cookie
      .split("; ")
      .find((x) => x.startsWith(name + "="))
      ?.split("=")[1] || ""
  );
}

function errorMessage(data: unknown, fallback: string): string {
  if (typeof data === "string" && data.trim()) return data.trim();
  if (!data || typeof data !== "object") return fallback;
  const record = data as Record<string, unknown>;
  if (typeof record.detail === "string" && record.detail.trim())
    return record.detail.trim();
  const parts: string[] = [];
  for (const [key, value] of Object.entries(record)) {
    const values = Array.isArray(value) ? value : [value];
    for (const item of values) {
      if (typeof item === "string" && item.trim())
        parts.push(`${key}: ${item.trim()}`);
      else if (item && typeof item === "object")
        parts.push(`${key}: ${JSON.stringify(item)}`);
    }
  }
  return parts.join(" · ") || fallback;
}

export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
    public code?: string,
  ) {
    super(message);
  }
}
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method || "GET").toUpperCase();
  const headers = new Headers(init.headers || {});
  headers.set("Accept", "application/json");
  if (
    init.body &&
    !(init.body instanceof FormData) &&
    !headers.has("Content-Type")
  )
    headers.set("Content-Type", "application/json");
  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    if (!cookie("csrftoken"))
      await fetch(`${API_BASE}/auth/csrf`, { credentials: "include" });
    headers.set("X-CSRFToken", decodeURIComponent(cookie("csrftoken")));
  }
  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      ...init,
      headers,
      credentials: "include",
    });
  } catch (error) {
    if (init.signal?.aborted) throw error;
    const reason =
      error instanceof Error ? error.message : "Network request failed";
    throw new Error(`Cannot reach VeloRay API. ${reason}`);
  }
  if (res.status === 204) return undefined as T;
  const ct = res.headers.get("content-type") || "";
  const data = ct.includes("json") ? await res.json() : await res.text();
  if (!res.ok) {
    const code =
      data && typeof data === "object"
        ? (data as { code?: string }).code
        : undefined;
    if (res.status === 401 && !path.startsWith("/auth/login"))
      window.dispatchEvent(new Event("veloray:session-expired"));
    if (code === "two_factor_setup_required")
      window.dispatchEvent(new Event("veloray:session-update"));
    throw new APIError(
      localizeError(errorMessage(data, `${res.status} ${res.statusText}`)),
      res.status,
      code,
    );
  }
  if (
    method === "GET" &&
    Array.isArray(data) &&
    !new URLSearchParams(path.split("?")[1] || "").has("offset")
  ) {
    const total = Number(res.headers.get("X-Total-Count") || data.length),
      limit = Number(res.headers.get("X-Page-Limit") || 250),
      result = [...data];
    for (let offset = data.length; offset < total; offset += limit) {
      const next = await api<unknown[]>(
        `${path}${path.includes("?") ? "&" : "?"}offset=${offset}&limit=${limit}`,
        init,
      );
      result.push(...next);
      if (next.length === 0) break;
    }
    return result as T;
  }
  return data as T;
}

export type Session = {
  role?: string;
  permissions?: string[];
  own_clients_only?: boolean;
  id: number;
  username: string;
  is_staff?: boolean;
  is_superuser?: boolean;
  two_factor_setup_required?: boolean;
};
export async function currentSession() {
  return api<Session>("/auth/me");
}
export type LoginResult = Session | { two_factor_required: true };
export async function login(
  username: string,
  password: string,
  totp_code = "",
  provider: "local" | "ldap" = "local",
) {
  await api("/auth/csrf");
  return api<LoginResult>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ username, password, totp_code, provider }),
  });
}
export async function logout() {
  return api("/auth/logout", { method: "POST" });
}

export type NodeItem = {
  id: number;
  name: string;
  public_host: string;
  agent_url: string;
  enabled: boolean;
  verify_tls: boolean;
  status: string;
  xray_version: string;
  last_seen_at: string | null;
  deploy_state: string;
  last_deploy_error: string;
  is_local: boolean;
  config_patch: Record<string, unknown>;
};
export type InboundItem = {
  core?: "xray" | "singbox" | "native";
  id: number;
  node: number;
  node_name: string;
  name: string;
  listen: string;
  port: number;
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
  protocol_settings: Record<string, any>;
  stream_settings: Record<string, unknown>;
  enabled: boolean;
  client_count: number;
  supports_clients: boolean;
};
export type ClientItem = {
  hwid_limit?: number;
  device_limit: number;
  device_count: number;
  start_after_first_use: boolean;
  duration_days: number;
  first_used_at: string | null;
  owner_id: number;
  online: boolean | null;
  connection_status: string;
  telemetry_at?: string;
  id: number;
  inbound: number;
  inbound_name: string;
  node_name: string;
  name: string;
  credential: string;
  enabled: boolean;
  expires_at: string | null;
  traffic_limit_bytes: number;
  used_traffic_bytes: number;
  raw_used_traffic_bytes: number;
  raw_lifetime_traffic_bytes: number;
  raw_upload_traffic_bytes: number;
  raw_download_traffic_bytes: number;
  traffic_multiplier: number;
  effective_traffic_multiplier: number;
  account_id: number;
  account_connections?: number;
  last_traffic_at: string | null;
  disabled_reason: string;
  transport: string;
  security: string;
  usage_metered: boolean;
  renewal_interval_days: number;
  next_renewal_at: string | null;
  note: string;
  protocol: string;
  protocol_settings: Record<string, any>;
  share_link: string;
  profile_text: string;
  subscription_url: string;
  subscription_portal_url: string;
};
export type SubscriptionHost = {
  name: string;
  inbound: number;
  address: string;
  port: number;
  sni: string;
  path: string;
  host_header: string;
  enabled: boolean;
};
export type AppSettings = {
  smtp_enabled: boolean;
  smtp_host: string;
  smtp_port: number;
  smtp_tls_mode: string;
  smtp_username: string;
  smtp_from: string;
  smtp_configured: boolean;
  alert_quota_percent: number;
  alert_expiry_days: number;
  alert_inactive_days: number;
  subscription_hosts: SubscriptionHost[];
  site_name: string;
  support_url: string;
  announcement: string;
  announcement_url: string;
  default_locale: string;
  default_client_days: number;
  default_traffic_gb: number;
  default_renewal_days: number;
  traffic_history_days: number;
  dashboard_refresh_seconds: number;
  subscription_enabled: boolean;
  subscription_show_apps: boolean;
  subscription_show_qr: boolean;
  subscription_show_connection_uri: boolean;
  subscription_footer: string;
  subscription_template: "default" | "compact" | "dashboard" | "custom";
  subscription_template_path: string;
  subscription_hwid_required: boolean;
  subscription_hwid_limit: number;
  subscription_clash_template: string;
  subscription_singbox_template: string;
  subscription_xray_template: string;
  subscription_update_interval_hours: number;
  subscription_default_format:
    | "auto"
    | "base64"
    | "raw"
    | "clash"
    | "singbox"
    | "xray";
  subscription_show_analytics: boolean;
  subscription_show_downloads: boolean;
  subscription_theme: "system" | "light" | "dark";
  maintenance_mode: boolean;
  require_2fa_for_admins: boolean;
  api_keys_enabled: boolean;
  api_key_default_days: number;
  audit_retention_days: number;
  accent: string;
  telegram_enabled: boolean;
  telegram_locale: "en" | "fa";
  telegram_allow_changes: boolean;
  telegram_client_alerts: boolean;
  telegram_quota_warning_percent: number;
  telegram_expiry_warning_days: number;
  telegram_configured: boolean;
  webhook_enabled: boolean;
  webhook_url: string;
  webhook_configured: boolean;
  telegram_owner_ids: string[];
  telegram_alert_cpu_percent: number;
  telegram_alert_memory_percent: number;
  telegram_alert_disk_percent: number;
  telegram_alert_cooldown_minutes: number;
};
export type AdminUser = {
  ldap_dn?: string;
  oidc_issuer?: string;
  oidc_subject?: string;
  role: "administrator" | "reseller" | "viewer" | "custom";
  permissions: string[];
  allowed_inbounds: number[];
  own_clients_only: boolean;
  max_clients: number;
  traffic_budget_bytes: number;
  clients_count: number;
  allocated_traffic_bytes: number;
  id: number;
  username: string;
  email: string;
  is_active: boolean;
  is_staff: boolean;
  is_superuser: boolean;
  last_login: string | null;
  date_joined: string;
  two_factor_enabled: boolean;
};
export type SystemInfo = {
  panel_uptime_seconds?: number;
  panel_started_at?: string;
  hostname: string;
  platform: string;
  go_version: string;
  uptime_seconds: number;
  load_average: number[];
  cpu_percent: number;
  cpu_count: number;
  memory_total_bytes: number;
  memory_used_bytes: number;
  memory_available_bytes: number;
  memory_percent: number;
  swap_total_bytes: number;
  swap_used_bytes: number;
  swap_free_bytes: number;
  swap_percent: number;
  disk_total_bytes: number;
  disk_used_bytes: number;
  disk_free_bytes: number;
  disk_percent: number;
  users_total: number;
  admins_total: number;
};

export type AdminAPIKey = {
  id: number;
  name: string;
  prefix: string;
  scope: "read" | "write" | "admin";
  enabled: boolean;
  expires_at: string | null;
  last_used_at: string | null;
  user: number;
  user_name: string;
  created_at: string;
  updated_at: string;
  token?: string;
};

export function distinctAccounts(clients: ClientItem[]): ClientItem[] {
  const seen = new Set<string>();
  return clients.filter((c) => {
    const key = c.account_id ? `account:${c.account_id}` : `client:${c.id}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}
export function effectiveMultiplier(value: number): number {
  return value < 0 ? 1 + value : value;
}
export type ImportPreview = {
  token: string;
  source: string;
  node: number;
  node_name: string;
  accounts: number;
  client_links: number;
  used_traffic_bytes: number;
  warnings: string[];
  inbounds: {
    source_id: string;
    name: string;
    port: number;
    protocol: string;
    transport?: string;
    security?: string;
    supported: boolean;
    client_count: number;
    warnings: string[];
    existing_id: number;
    conflict_id: number;
    port_state?: string;
    port_owner?: string;
  }[];
};
export type ImportResult = {
  status: string;
  inbounds_created: number;
  client_links_created: number;
  shared_accounts: number;
  inbounds_skipped: number;
  inbound_ids: number[];
};
export type TelegramStatus = {
  username: string;
  webhook_configured: boolean;
  owner_count: number;
  last_success: string;
  last_error: string;
};

function localizeError(message: string): string {
  if (localeName() !== "fa-IR") return message;
  const translated = t(message);
  if (translated !== message) return translated;
  return "درخواست انجام نشد. ورودی‌ها و دسترسی حساب را بررسی کنید.";
}

export type ClientIPList = {
  count: number;
  available: boolean;
  observed_at: string;
  items: {
    ip: string;
    last_seen_at: string;
    connections: {
      node: number;
      node_name: string;
      inbound: number;
      inbound_name: string;
      core: string;
    }[];
  }[];
};
export type ClientDeviceList = {
  count: number;
  items: {
    id: number;
    hwid: string;
    revoked?: boolean;
    device_model?: string;
    device_os?: string;
    os_version?: string;
    user_agent?: string;
    first_seen_at: string;
    last_seen_at: string;
  }[];
};
