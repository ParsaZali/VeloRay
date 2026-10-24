import { lazy, Suspense } from "react";
import { notify } from "@/lib/notifications";
import { PanelUpdate } from "@/components/panel-update";
import { PanelEndpoint } from "@/components/panel-endpoint";
import { SubscriptionEndpoint } from "@/components/subscription-endpoint";
import { SMTPSettings } from "@/components/smtp-settings";
import { DatePreferences } from "@/components/date-preferences";
import { NotificationEndpoints } from "@/components/notification-endpoints";
import { SubscriptionOptions } from "@/components/subscription-options";
import { useSession } from "@/lib/session";
import { Textarea } from "@/components/ui/controls";
import { ImportsPage } from "@/pages/imports";
import { Select, Input } from "@/components/ui/controls";
import { t } from "@/lib/i18n";
import { useLocale, tx } from "@/lib/i18n";
import {
  FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  api,
  type AppSettings,
  type SystemInfo,
  type TelegramStatus,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/common/page-header";
import { FaIcon } from "@/components/ui/fa-icon";

const ProfilesPage = lazy(() =>
  import("@/pages/profiles").then((m) => ({ default: m.ProfilesPage })),
);
const SubscriptionPoliciesPage = lazy(() =>
  import("@/pages/subscription-policies").then((m) => ({
    default: m.SubscriptionPoliciesPage,
  })),
);

const fmt = (b: number) => {
  if (!b) return t("0 B");
  const u = ["B", t("KB"), t("MB"), t("GB"), t("TB")];
  const i = Math.max(
    0,
    Math.min(Math.floor(Math.log(b) / Math.log(1024)), u.length - 1),
  );
  return `${(b / 1024 ** i).toFixed(i < 2 ? 0 : 1)} ${u[i]}`;
};
const duration = (s: number) => {
  const d = Math.floor(s / 86400),
    h = Math.floor((s % 86400) / 3600),
    m = Math.floor((s % 3600) / 60);
  return d
    ? tx("{0}d {1}h", d, h)
    : h
      ? tx("{0}h {1}m", h, m)
      : s >= 60
        ? tx("{0}m", m)
        : tx("{0}s", Math.floor(s));
};
type Health = {
  status: string;
  version: string;
  database?: string;
  uptime_seconds?: number;
  started_at?: string;
};
const defaults: AppSettings = {
  subscription_hosts: [],
  site_name: "VeloRay",
  support_url: "",
  announcement: "",
  announcement_url: "",
  default_locale: "en",
  default_client_days: 30,
  default_traffic_gb: 0,
  default_renewal_days: 0,
  traffic_history_days: 30,
  dashboard_refresh_seconds: 30,
  subscription_enabled: true,
  subscription_path: "/sub/",
  subscription_show_apps: true,
  subscription_show_qr: true,
  subscription_show_connection_uri: true,
  subscription_footer: "",
  subscription_template: "default",
  subscription_template_path: "",
  subscription_hwid_required: false,
  subscription_hwid_limit: 0,
  subscription_clash_template: "",
  subscription_singbox_template: "",
  subscription_xray_template: "",
  subscription_update_interval_hours: 12,
  subscription_default_format: "auto",
  subscription_show_analytics: true,
  subscription_show_downloads: true,
  subscription_theme: "system",
  maintenance_mode: false,
  require_2fa_for_admins: false,
  passkeys_enabled: true,
  api_keys_enabled: true,
  api_key_default_days: 90,
  audit_retention_days: 90,
  accent: "emerald",
  telegram_enabled: false,
  telegram_locale: "en",
  telegram_allow_changes: false,
  telegram_client_alerts: true,
  telegram_quota_warning_percent: 80,
  telegram_expiry_warning_days: 3,
  telegram_configured: false,
  smtp_enabled: false,
  smtp_host: "",
  smtp_port: 465,
  smtp_tls_mode: "tls",
  smtp_username: "",
  smtp_from: "",
  smtp_configured: false,
  alert_quota_percent: 0,
  alert_expiry_days: 0,
  alert_inactive_days: 0,
  webhook_enabled: false,
  webhook_url: "",
  webhook_configured: false,
  telegram_owner_ids: [],
  telegram_alert_cpu_percent: 90,
  telegram_alert_memory_percent: 90,
  telegram_alert_disk_percent: 90,
  telegram_alert_cooldown_minutes: 30,
};

type Tab =
  | "imports"
  | "profiles"
  | "subscription-policies"
  | "general"
  | "subscription"
  | "defaults"
  | "security"
  | "telegram"
  | "webhooks"
  | "system";
export function SettingsPage({ initialTab = "general" }: { initialTab?: Tab }) {
  useLocale();
  const [h, setH] = useState<Health | null>(null),
    [cfg, setCfg] = useState<AppSettings>(defaults),
    [sys, setSys] = useState<SystemInfo | null>(null),
    [canEdit, setCanEdit] = useState(false),
    [tab, setTab] = useState<Tab>(initialTab),
    [msg, setMsg] = useState(""),
    [busy, setBusy] = useState(false),
    [telegramToken, setTelegramToken] = useState(""),
    [ownerIDs, setOwnerIDs] = useState(""),
    [clearTelegram, setClearTelegram] = useState(false),
    [smtpPassword, setSmtpPassword] = useState(""),
    [clearSMTP, setClearSMTP] = useState(false),
    [webhookSecret, setWebhookSecret] = useState(""),
    [clearWebhook, setClearWebhook] = useState(false),
    [webhookStats, setWebhookStats] = useState<{
      pending: number;
      failed: number;
      delivered: number;
    } | null>(null);
  useEffect(() => setTab(initialTab), [initialTab]);
  const session = useSession();
  const canReadSystem = Boolean(
    session?.is_superuser || session?.permissions?.includes("nodes.read"),
  );
  const request = useRef<AbortController | null>(null);
  const [loading, setLoading] = useState(true),
    [healthSeenAt, setHealthSeenAt] = useState(Date.now()),
    [clock, setClock] = useState(Date.now());
  const load = useCallback(async () => {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const timeout = setTimeout(() => controller.abort(), 15000);
    setLoading(true);
    setMsg("");
    setCanEdit(Boolean(session?.is_superuser));
    const current = () => request.current === controller;
    const results = await Promise.allSettled([
      api<Health>("/health", { signal: controller.signal })
        .then((value) => {
          if (current()) {
            setH(value);
            setHealthSeenAt(Date.now());
          }
        })
        .catch((error) => {
          if (current()) setH(null);
          throw error;
        }),
      api<AppSettings>("/settings", { signal: controller.signal }).then(
        (value) => {
          if (current()) {
            setCfg({ ...defaults, ...value });
            setOwnerIDs((value.telegram_owner_ids || []).join(", "));
          }
        },
      ),
      canReadSystem
        ? api<SystemInfo>("/system-info", { signal: controller.signal })
            .then((value) => {
              if (current()) setSys(value);
            })
            .catch((error) => {
              if (current()) setSys(null);
              throw error;
            })
        : Promise.resolve(null),
    ]);
    clearTimeout(timeout);
    if (current()) {
      setLoading(false);
      const failed = results.filter((result) => result.status === "rejected");
      if (failed.length) setMsg(t("Some system information is unavailable"));
    }
  }, [session?.is_superuser, canReadSystem]);
  useEffect(() => {
    void load();
    return () => {
      request.current?.abort();
      request.current = null;
    };
  }, [load]);
  useEffect(() => {
    let active = true,
      controller: AbortController | null = null;
    const clockTimer = setInterval(() => setClock(Date.now()), 1000);
    const healthTimer = setInterval(async () => {
      if (document.hidden || controller) return;
      controller = new AbortController();
      const timeout = setTimeout(() => controller?.abort(), 10000);
      try {
        const value = await api<Health>("/health", {
          signal: controller.signal,
        });
        if (active) {
          setH(value);
          setHealthSeenAt(Date.now());
        }
      } catch {
        if (active) setH(null);
      } finally {
        clearTimeout(timeout);
        controller = null;
      }
    }, 30000);
    return () => {
      active = false;
      clearInterval(clockTimer);
      clearInterval(healthTimer);
      controller?.abort();
    };
  }, []);
  const [botStatus, setBotStatus] = useState<TelegramStatus | null>(null);
  async function checkBot(action: "status" | "test" | "setup") {
    setBusy(true);
    setMsg("");
    try {
      const result = await api<TelegramStatus>(`/telegram/${action}`, {
        method: action === "status" ? "GET" : "POST",
      });
      setBotStatus(result);
      notify(
        action === "status"
          ? t("Bot connection checked.")
          : action === "test"
            ? t("Test delivered to configured owners.")
            : t("Owner command menu registered."),
        "success",
      );
    } catch (e) {
      notify(e instanceof Error ? e.message : t("Bot check failed"), "error");
    } finally {
      setBusy(false);
    }
  }
  async function webhookAction(action: "status" | "test" | "retry") {
    setBusy(true);
    setMsg("");
    try {
      const data = await api<{
        pending?: number;
        failed?: number;
        delivered?: number;
        queued?: boolean;
        retried?: number;
      }>(`/webhooks/${action}`, {
        method: action === "status" ? "GET" : "POST",
      });
      if (action === "status") {
        setWebhookStats({
          pending: data.pending || 0,
          failed: data.failed || 0,
          delivered: data.delivered || 0,
        });
        setMsg(t("Webhook status loaded."));
      } else {
        setMsg(
          action === "test"
            ? t("Signed test event added to delivery queue.")
            : t("Failed webhook deliveries scheduled for retry."),
        );
        const status = await api<{
          pending: number;
          failed: number;
          delivered: number;
        }>("/webhooks/status");
        setWebhookStats(status);
      }
    } catch (e) {
      setMsg(e instanceof Error ? e.message : t("Webhook operation failed"));
    } finally {
      setBusy(false);
    }
  }
  const diskPct = useMemo(() => Math.round(sys?.disk_percent ?? 0), [sys]);
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!canEdit) {
      setMsg(t("Settings are read-only for non-superuser operators."));
      return;
    }
    setBusy(true);
    setMsg("");
    try {
      const ids = ownerIDs
        .split(/[,،;\s]+/)
        .map((id) => id.trim())
        .filter(Boolean);
      if (ids.some((id) => !/^[1-9][0-9]{0,18}$/.test(id)))
        throw new Error(
          t(
            "Enter numeric Telegram user IDs separated by commas.",
            "شناسه‌های عددی تلگرام را با ویرگول جدا کنید.",
          ),
        );
      const payload: any = { ...cfg, telegram_owner_ids: [...new Set(ids)] };
      delete payload.telegram_configured;
      delete payload.webhook_configured;
      delete payload.smtp_configured;
      if (smtpPassword) payload.smtp_password = smtpPassword;
      if (clearSMTP) payload.smtp_clear_password = true;
      if (webhookSecret.trim())
        payload.webhook_signing_secret = webhookSecret.trim();
      if (clearWebhook) payload.webhook_clear_secret = true;
      if (telegramToken.trim())
        payload.telegram_bot_token = telegramToken.trim();
      if (clearTelegram) payload.telegram_clear_bot_token = true;
      const next = await api<AppSettings>("/settings", {
        method: "PATCH",
        body: JSON.stringify(payload),
      });
      setCfg({ ...defaults, ...next });
      setOwnerIDs((next.telegram_owner_ids || []).join(", "));
      setTelegramToken("");
      setClearTelegram(false);
      setWebhookSecret("");
      setSmtpPassword("");
      setClearSMTP(false);
      setClearWebhook(false);
      notify(t("Settings saved."), "success");
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  const set = (k: keyof AppSettings, v: any) =>
    setCfg((c) => ({ ...c, [k]: v }));
  const nav: [Tab, string, string][] = [
    ["general", t("General"), "gear"],
    ["subscription", t("Subscription"), "rss"],
    ...(session?.is_superuser
      ? ([
          ["subscription-policies", t("Subscription policies"), "link"],
          ["profiles", t("Config profiles"), "file-code"],
        ] as [Tab, string, string][])
      : []),
    ["defaults", t("Client defaults"), "sliders"],
    ["security", t("Security & API"), "shield-halved"],
    ["telegram", t("Telegram"), "paper-plane"],
    ["webhooks", t("Webhooks"), "code-branch"],
    ["system", t("System"), "server"],
    ...(canEdit
      ? ([["imports", t("Import data"), "download"]] as [Tab, string, string][])
      : []),
  ];
  return (
    <div className="settings-page space-y-6">
      <PageHeader
        title={t("Settings")}
        actions={
          <Button variant="outline" onClick={load}>
            {t("Refresh")}
          </Button>
        }
      />
      {msg && (
        <div
          role="status"
          aria-live="polite"
          className="rounded-lg border border-[var(--border)] bg-[var(--card)] px-4 py-3 text-[11px] text-[var(--muted-strong)]"
        >
          {msg}
        </div>
      )}
      <div className="settings-status">
        <Info
          icon="code-branch"
          label={t("VeloRay version")}
          value={h?.version || "1.0"}
        />
        <Info
          icon="database"
          label={t("Database")}
          value={
            (h?.database ? h.database === "ok" : h?.status === "ok")
              ? t("Connected")
              : loading && !h
                ? t("Checking…")
                : t("Unavailable")
          }
        />
        <Info
          icon="server"
          label={t("Host")}
          value={
            sys?.hostname ||
            (canReadSystem
              ? loading
                ? t("Checking…")
                : t("Unavailable")
              : "—")
          }
        />
        <Info
          icon="clock"
          label={t("Uptime")}
          value={
            typeof h?.uptime_seconds === "number"
              ? duration(
                  h.uptime_seconds + Math.max(0, (clock - healthSeenAt) / 1000),
                )
              : typeof sys?.panel_uptime_seconds === "number"
                ? duration(sys.panel_uptime_seconds)
                : loading
                  ? t("Checking…")
                  : t("Unavailable")
          }
        />
      </div>
      {!canEdit && (
        <div className="rounded-lg border border-amber-500/20 bg-amber-500/8 px-4 py-3 text-[11px] text-amber-600 dark:text-amber-400">
          <FaIcon icon="lock" className="mr-2" />
          {t(
            "Read-only operator view. Product settings can only be changed by a superuser.",
          )}
        </div>
      )}
      <div className="settings-layout">
        <nav
          className="settings-nav"
          aria-label={t("Settings sections", "بخش‌های تنظیمات")}
        >
          <div className="settings-nav-inner">
            {nav.map(([k, l, i]) => (
              <button
                key={k}
                onClick={() => setTab(k)}
                aria-pressed={tab === k}
                className={`flex w-full items-center gap-2 rounded-md px-3 py-2.5 text-left text-[11px] font-medium transition ${tab === k ? "bg-[var(--surface-2)] text-[var(--fg)]" : "text-[var(--muted-strong)] hover:bg-[var(--surface-2)]"}`}
              >
                <FaIcon icon={i} />
                {l}
              </button>
            ))}
          </div>
        </nav>
        {tab === "profiles" && session?.is_superuser ? (
          <Suspense fallback={<p role="status">{t("Loading…")}</p>}>
            <ProfilesPage />
          </Suspense>
        ) : tab === "subscription-policies" && session?.is_superuser ? (
          <Suspense fallback={<p role="status">{t("Loading…")}</p>}>
            <SubscriptionPoliciesPage />
          </Suspense>
        ) : tab === "imports" ? (
          <ImportsPage embedded />
        ) : (
          <div className="settings-content min-w-0 space-y-4">
            {tab === "general" && canEdit && !loading && <PanelEndpoint />}
            <SubscriptionEndpoint
              enabled={tab === "subscription" && canEdit && !loading}
            />
            <form onSubmit={save}>
              <fieldset disabled={!canEdit} className="contents">
                {tab === "general" && (
                  <Card>
                    <CardHeader>
                      <div>
                        <CardTitle>{t("General")}</CardTitle>
                      </div>
                    </CardHeader>
                    <CardContent className="grid gap-4 sm:grid-cols-2">
                      <Field label={t("Panel name")}>
                        <Input
                          className="input"
                          value={cfg.site_name}
                          onChange={(e) => set("site_name", e.target.value)}
                        />
                      </Field>
                      <Field label={t("Default locale")}>
                        <Select
                          className="input"
                          value={cfg.default_locale}
                          onChange={(e) =>
                            set("default_locale", e.target.value)
                          }
                        >
                          <option value="en">{t("English")}</option>
                          <option value="fa">{t("Persian")}</option>
                        </Select>
                      </Field>
                      <Field label={t("Support URL")} className="sm:col-span-2">
                        <Input
                          className="input"
                          value={cfg.support_url}
                          onChange={(e) => set("support_url", e.target.value)}
                          placeholder={t("https://...")}
                        />
                      </Field>
                      <Field
                        label={t("Announcement")}
                        className="sm:col-span-2"
                      >
                        <Textarea
                          className="input min-h-24"
                          value={cfg.announcement}
                          onChange={(e) => set("announcement", e.target.value)}
                          placeholder={t(
                            "Message sent to compatible subscription clients",
                          )}
                        />
                      </Field>
                      <Field
                        label={t("Announcement URL")}
                        className="sm:col-span-2"
                      >
                        <Input
                          className="input"
                          value={cfg.announcement_url}
                          onChange={(e) =>
                            set("announcement_url", e.target.value)
                          }
                          placeholder={t("https://...")}
                        />
                      </Field>
                      <Field label={t("Dashboard refresh")}>
                        <Select
                          className="input"
                          value={cfg.dashboard_refresh_seconds}
                          onChange={(e) =>
                            set(
                              "dashboard_refresh_seconds",
                              Number(e.target.value),
                            )
                          }
                        >
                          {[10, 15, 30, 60, 120, 300].map((x) => (
                            <option key={x} value={x}>
                              {x}
                              {t("s")}
                            </option>
                          ))}
                        </Select>
                      </Field>
                      <Toggle
                        label={t("Maintenance mode")}
                        text={t(
                          "Pause public subscriptions while administrator access remains online.",
                        )}
                        checked={cfg.maintenance_mode}
                        onChange={(v) => set("maintenance_mode", v)}
                      />
                      <Actions busy={busy} />
                    </CardContent>
                  </Card>
                )}
                {tab === "subscription" && (
                  <Card>
                    <CardHeader>
                      <div>
                        <CardTitle>{t("Subscription portal")}</CardTitle>
                      </div>
                    </CardHeader>
                    <CardContent className="grid gap-4 sm:grid-cols-2">
                      <div className="settings-section-title sm:col-span-2">
                        <h3>{t("Subscription page", "صفحهٔ اشتراک")}</h3>
                      </div>
                      <Field label={t("Template")}>
                        <Select
                          className="input"
                          value={
                            cfg.subscription_template === "custom"
                              ? "custom"
                              : "default"
                          }
                          onChange={(e) =>
                            set("subscription_template", e.target.value)
                          }
                        >
                          <option value="default">
                            {t("Simple subscription page")}
                          </option>
                          <option value="custom">
                            {t("Custom HTML file")}
                          </option>
                        </Select>
                      </Field>
                      <Field label={t("Subscription path", "مسیر اشتراک")}>
                        <Input
                          className="input"
                          dir="ltr"
                          value={cfg.subscription_path}
                          placeholder="/sub/"
                          onChange={(e) =>
                            set("subscription_path", e.target.value)
                          }
                        />
                        <span className="field-hint">
                          {t(
                            "Leave empty for /TOKEN. Changing this path changes existing links.",
                            "برای /TOKEN خالی بگذارید. تغییر مسیر، آدرس لینک‌های قبلی را عوض می‌کند.",
                          )}
                        </span>
                      </Field>
                      <SubscriptionOptions
                        cfg={cfg}
                        set={(key, value) =>
                          setCfg((previous) => ({ ...previous, [key]: value }))
                        }
                        disabled={!canEdit || busy}
                      />
                      <Field label={t("Portal footer")}>
                        <Input
                          className="input"
                          maxLength={180}
                          value={cfg.subscription_footer}
                          onChange={(e) =>
                            set("subscription_footer", e.target.value)
                          }
                          placeholder={t("Optional footer")}
                        />
                      </Field>
                      <Field label={t("Subscription update interval (hours)")}>
                        <Input
                          className="input"
                          type="number"
                          min={1}
                          max={168}
                          value={cfg.subscription_update_interval_hours}
                          onChange={(e) =>
                            set(
                              "subscription_update_interval_hours",
                              Number(e.target.value),
                            )
                          }
                        />
                      </Field>
                      <Field label={t("Default subscription format")}>
                        <Select
                          className="input"
                          value={cfg.subscription_default_format}
                          onChange={(e) =>
                            set("subscription_default_format", e.target.value)
                          }
                        >
                          <option value="auto">{t("Auto detect")}</option>
                          <option value="base64">Base64</option>
                          <option value="raw">RAW</option>
                          <option value="clash">Clash</option>
                          <option value="singbox">sing-box</option>
                          <option value="xray">Xray JSON</option>
                        </Select>
                      </Field>
                      <Field label={t("Subscription portal theme")}>
                        <Select
                          className="input"
                          value={cfg.subscription_theme}
                          onChange={(e) =>
                            set("subscription_theme", e.target.value)
                          }
                        >
                          <option value="system">{t("Follow device")}</option>
                          <option value="light">{t("Light")}</option>
                          <option value="dark">{t("Dark")}</option>
                        </Select>
                      </Field>
                      <Toggle
                        label={t("Traffic chart")}
                        text={t(
                          "Display actual historical traffic in the subscription portal.",
                        )}
                        checked={cfg.subscription_show_analytics}
                        onChange={(v) => set("subscription_show_analytics", v)}
                      />
                      <Toggle
                        label={t("Subscription downloads")}
                        text={t(
                          "Show export buttons for supported client configuration formats.",
                        )}
                        checked={cfg.subscription_show_downloads}
                        onChange={(v) => set("subscription_show_downloads", v)}
                      />
                      <Toggle
                        label={t("Subscriptions enabled")}
                        text={t(
                          "Turn public subscription endpoints on or off.",
                        )}
                        checked={cfg.subscription_enabled}
                        onChange={(v) => set("subscription_enabled", v)}
                      />
                      <Toggle
                        label={t("Recommended apps")}
                        text={t("Display compatible client suggestions.")}
                        checked={cfg.subscription_show_apps}
                        onChange={(v) => set("subscription_show_apps", v)}
                      />
                      <Toggle
                        label={t("QR code")}
                        text={t("Allow subscription URL QR display.")}
                        checked={cfg.subscription_show_qr}
                        onChange={(v) => set("subscription_show_qr", v)}
                      />
                      <Toggle
                        label={t("Connection URI")}
                        text={t("Expose the raw protocol URI when supported.")}
                        checked={cfg.subscription_show_connection_uri}
                        onChange={(v) =>
                          set("subscription_show_connection_uri", v)
                        }
                      />
                      <Actions busy={busy} />
                    </CardContent>
                  </Card>
                )}
                {tab === "defaults" && (
                  <Card>
                    <CardHeader>
                      <div>
                        <CardTitle>{t("Client defaults")}</CardTitle>
                      </div>
                    </CardHeader>
                    <CardContent className="grid gap-4 sm:grid-cols-2">
                      <Field label={t("Default expiry (days)")}>
                        <Input
                          className="input"
                          type="number"
                          min="0"
                          value={cfg.default_client_days}
                          onChange={(e) =>
                            set("default_client_days", Number(e.target.value))
                          }
                        />
                      </Field>
                      <Field label={t("Default traffic (GB)")}>
                        <Input
                          className="input"
                          type="number"
                          min="0"
                          value={cfg.default_traffic_gb}
                          onChange={(e) =>
                            set("default_traffic_gb", Number(e.target.value))
                          }
                        />
                      </Field>
                      <Field label={t("Default renewal (days)")}>
                        <Input
                          className="input"
                          type="number"
                          min="0"
                          value={cfg.default_renewal_days}
                          onChange={(e) =>
                            set("default_renewal_days", Number(e.target.value))
                          }
                        />
                      </Field>
                      <Field label={t("Traffic history retention")}>
                        <Select
                          className="input"
                          value={cfg.traffic_history_days}
                          onChange={(e) =>
                            set("traffic_history_days", Number(e.target.value))
                          }
                        >
                          {[7, 14, 30, 60, 90, 180, 365].map((x) => (
                            <option key={x} value={x}>
                              {x} {t("days")}
                            </option>
                          ))}
                        </Select>
                      </Field>
                      <Actions busy={busy} />
                    </CardContent>
                  </Card>
                )}
                {tab === "security" && (
                  <Card>
                    <CardHeader>
                      <div>
                        <CardTitle>{t("Security & API")}</CardTitle>
                      </div>
                    </CardHeader>
                    <CardContent className="grid gap-4 sm:grid-cols-2">
                      <Toggle
                        label={t("Require administrator 2FA")}
                        text={t(
                          "Staff must use TOTP or a verified passkey before protected API use.",
                          "کارکنان باید از TOTP یا کلید ورود تأییدشده استفاده کنند.",
                        )}
                        checked={cfg.require_2fa_for_admins}
                        onChange={(v) => set("require_2fa_for_admins", v)}
                      />
                      <Toggle
                        label={t("Enable passkeys", "فعال‌سازی کلید ورود")}
                        text={t(
                          "Allow device PIN, biometrics and security keys on your trusted HTTPS domain.",
                          "ورود با پین دستگاه، تشخیص بیومتریک و کلید امنیتی روی دامنهٔ HTTPS معتبر.",
                        )}
                        checked={cfg.passkeys_enabled}
                        onChange={(v) => set("passkeys_enabled", v)}
                      />
                      <Toggle
                        label={t("Enable API keys")}
                        text={t("Allow scoped Bearer tokens from Accounts.")}
                        checked={cfg.api_keys_enabled}
                        onChange={(v) => set("api_keys_enabled", v)}
                      />
                      <Field label={t("Default API key lifetime")}>
                        <Input
                          className="input"
                          type="number"
                          min="1"
                          max="3650"
                          value={cfg.api_key_default_days}
                          onChange={(e) =>
                            set("api_key_default_days", Number(e.target.value))
                          }
                        />
                      </Field>
                      <Field label={t("Audit retention")}>
                        <Input
                          className="input"
                          type="number"
                          min="1"
                          max="3650"
                          value={cfg.audit_retention_days}
                          onChange={(e) =>
                            set("audit_retention_days", Number(e.target.value))
                          }
                        />
                      </Field>
                      <Actions busy={busy} />
                    </CardContent>
                  </Card>
                )}
                {tab === "telegram" && (
                  <Card>
                    <CardHeader>
                      <div>
                        <CardTitle>{t("Telegram automation")}</CardTitle>
                      </div>
                      <span
                        className={`rounded-full px-2 py-1 text-[9px] ${cfg.telegram_configured ? "bg-emerald-500/10 text-emerald-500" : "bg-[var(--surface-2)] text-[var(--muted)]"}`}
                      >
                        {cfg.telegram_configured
                          ? t("Token configured")
                          : t("No token")}
                      </span>
                    </CardHeader>
                    <CardContent className="grid gap-4 sm:grid-cols-2">
                      <Toggle
                        label={t("Telegram enabled")}
                        text={t(
                          "Send alerts and accept commands from configured owner IDs.",
                        )}
                        checked={cfg.telegram_enabled}
                        onChange={(v) => set("telegram_enabled", v)}
                      />
                      <Toggle
                        label={t("Allow bot management")}
                        text={t(
                          "Enable confirmed client and node changes. Required panel 2FA keeps bot management read-only.",
                        )}
                        checked={cfg.telegram_allow_changes}
                        onChange={(v) => set("telegram_allow_changes", v)}
                      />
                      <Toggle
                        label={t("Quota and expiry alerts")}
                        text={t(
                          "Notify owners before quota exhaustion or account expiry.",
                        )}
                        checked={cfg.telegram_client_alerts}
                        onChange={(v) => set("telegram_client_alerts", v)}
                      />
                      <Field label={t("Bot language")}>
                        <Select
                          className="input"
                          value={cfg.telegram_locale}
                          onChange={(e) =>
                            set("telegram_locale", e.target.value)
                          }
                        >
                          <option value="en">{t("English")}</option>
                          <option value="fa">{t("Persian")}</option>
                        </Select>
                      </Field>
                      <Field label={t("Quota warning %")}>
                        <Input
                          className="input"
                          type="number"
                          min="1"
                          max="100"
                          value={cfg.telegram_quota_warning_percent}
                          onChange={(e) =>
                            set(
                              "telegram_quota_warning_percent",
                              Number(e.target.value),
                            )
                          }
                        />
                      </Field>
                      <Field label={t("Expiry warning (days)")}>
                        <Input
                          className="input"
                          type="number"
                          min="1"
                          max="30"
                          value={cfg.telegram_expiry_warning_days}
                          onChange={(e) =>
                            set(
                              "telegram_expiry_warning_days",
                              Number(e.target.value),
                            )
                          }
                        />
                      </Field>
                      <Field label={t("Bot token")}>
                        <Input
                          className="input"
                          type="password"
                          autoComplete="off"
                          value={telegramToken}
                          onChange={(e) => setTelegramToken(e.target.value)}
                          placeholder={
                            cfg.telegram_configured
                              ? t("Leave blank to keep current token")
                              : "123456:ABC..."
                          }
                        />
                      </Field>
                      <Field label={t("Owner IDs")} className="sm:col-span-2">
                        <Input
                          className="input"
                          value={ownerIDs}
                          onChange={(e) => setOwnerIDs(e.target.value)}
                          dir="ltr"
                          placeholder="123456789, 987654321"
                        />
                      </Field>
                      <Field label={t("CPU alert %")}>
                        <Input
                          className="input"
                          type="number"
                          min="1"
                          max="100"
                          value={cfg.telegram_alert_cpu_percent}
                          onChange={(e) =>
                            set(
                              "telegram_alert_cpu_percent",
                              Number(e.target.value),
                            )
                          }
                        />
                      </Field>
                      <Field label={t("Memory alert %")}>
                        <Input
                          className="input"
                          type="number"
                          min="1"
                          max="100"
                          value={cfg.telegram_alert_memory_percent}
                          onChange={(e) =>
                            set(
                              "telegram_alert_memory_percent",
                              Number(e.target.value),
                            )
                          }
                        />
                      </Field>
                      <Field label={t("Disk alert %")}>
                        <Input
                          className="input"
                          type="number"
                          min="1"
                          max="100"
                          value={cfg.telegram_alert_disk_percent}
                          onChange={(e) =>
                            set(
                              "telegram_alert_disk_percent",
                              Number(e.target.value),
                            )
                          }
                        />
                      </Field>
                      <Field label={t("Alert cooldown (minutes)")}>
                        <Input
                          className="input"
                          type="number"
                          min="1"
                          max="1440"
                          value={cfg.telegram_alert_cooldown_minutes}
                          onChange={(e) =>
                            set(
                              "telegram_alert_cooldown_minutes",
                              Number(e.target.value),
                            )
                          }
                        />
                      </Field>
                      {cfg.telegram_configured && (
                        <Toggle
                          label={t("Remove stored bot token")}
                          text={t(
                            "Clear the encrypted Telegram credential on save.",
                          )}
                          checked={clearTelegram}
                          onChange={setClearTelegram}
                        />
                      )}
                      <div className="sm:col-span-2 rounded-lg border border-[var(--border)] bg-[var(--surface-2)] p-3 text-[10px] leading-5 text-[var(--muted)]">
                        {t("Commands:")} <code>{t("/status")}</code>,{" "}
                        <code>{t("/nodes")}</code>,{" "}
                        <code>{t("/clients [page]")}</code>,{" "}
                        <code>{t("/search name")}</code>,{" "}
                        <code>{t("/client ID")}</code>,{" "}
                        <code>{t("/link ID")}</code>
                        {t(
                          ". Management actions require a confirmation button that expires after five minutes. Every owner must start the bot before testing. Save changed settings before checking the connection.",
                        )}
                      </div>
                      <div className="sm:col-span-2 flex flex-wrap gap-2">
                        <Button
                          variant="outline"
                          disabled={
                            busy || !cfg.telegram_configured || !canEdit
                          }
                          onClick={() => checkBot("status")}
                        >
                          {t("Check bot connection")}
                        </Button>
                        <Button
                          variant="outline"
                          disabled={
                            busy || !cfg.telegram_configured || !canEdit
                          }
                          onClick={() => checkBot("test")}
                        >
                          {t("Send test to owners")}
                        </Button>
                        <Button
                          variant="outline"
                          disabled={
                            busy || !cfg.telegram_configured || !canEdit
                          }
                          onClick={() => checkBot("setup")}
                        >
                          {t("Register owner commands")}
                        </Button>
                      </div>
                      {botStatus && (
                        <div className="sm:col-span-2 rounded-md border border-[var(--border)] p-3 text-[12px] leading-6">
                          @{botStatus.username} · {botStatus.owner_count}{" "}
                          {t("owners ·")}{" "}
                          {botStatus.webhook_configured
                            ? t("Existing webhook blocks polling")
                            : t("Polling available")}
                          <br />
                          {t("Last successful poll:")}{" "}
                          {botStatus.last_success || t("Not yet")}
                          {botStatus.last_error && (
                            <p className="text-amber-500">
                              {botStatus.last_error}
                            </p>
                          )}
                        </div>
                      )}
                      <Actions busy={busy} />
                    </CardContent>
                  </Card>
                )}
                {tab === "webhooks" && (
                  <Card>
                    <CardHeader>
                      <CardTitle>{t("Signed event webhooks")}</CardTitle>
                    </CardHeader>
                    <CardContent className="grid gap-4 sm:grid-cols-2">
                      <Toggle
                        label={t("Enable webhooks")}
                        text={t(
                          "Deliver signed administrative audit events to your HTTPS endpoint. Save settings before sending a test.",
                        )}
                        checked={cfg.webhook_enabled}
                        onChange={(value) => set("webhook_enabled", value)}
                      />
                      <Field
                        label={t("HTTPS destination URL")}
                        className="sm:col-span-2"
                      >
                        <Input
                          className="input"
                          type="url"
                          placeholder="https://example.com/hooks/veloray"
                          value={cfg.webhook_url}
                          onChange={(e) => set("webhook_url", e.target.value)}
                        />
                      </Field>
                      <Field
                        label={t("HMAC signing secret")}
                        className="sm:col-span-2"
                      >
                        <Input
                          className="input"
                          type="password"
                          autoComplete="new-password"
                          value={webhookSecret}
                          onChange={(e) => setWebhookSecret(e.target.value)}
                          placeholder={
                            cfg.webhook_configured
                              ? t("Leave blank to keep the stored secret")
                              : t("At least 32 random characters")
                          }
                        />
                      </Field>
                      {cfg.webhook_configured && (
                        <Toggle
                          label={t("Remove stored signing secret")}
                          text={t(
                            "Disable webhooks before removing the secret.",
                          )}
                          checked={clearWebhook}
                          onChange={setClearWebhook}
                        />
                      )}
                      <div className="sm:col-span-2 rounded-lg border border-[var(--border)] p-3 text-[11px] leading-5 text-[var(--muted)]">
                        {t(
                          "Requests are signed using HMAC-SHA256 over timestamp + '.' + raw JSON body. Verify X-VeloRay-Timestamp and X-VeloRay-Signature. Event bodies exclude credentials, tokens, configuration and audit details. Failed deliveries are retried up to eight times.",
                        )}
                      </div>
                      {webhookStats && (
                        <div className="sm:col-span-2 rounded-lg border border-[var(--border)] p-3 text-[12px]">
                          {t("Queued")}: {webhookStats.pending} · {t("Failed")}:{" "}
                          {webhookStats.failed} · {t("Delivered (7 days)")}:{" "}
                          {webhookStats.delivered}
                        </div>
                      )}
                      <div className="sm:col-span-2 flex flex-wrap gap-2">
                        <Button
                          type="button"
                          variant="outline"
                          disabled={busy || !canEdit}
                          onClick={() => void webhookAction("status")}
                        >
                          {t("Queue status")}
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          disabled={
                            busy ||
                            !canEdit ||
                            !cfg.webhook_configured ||
                            !cfg.webhook_enabled
                          }
                          onClick={() => void webhookAction("test")}
                        >
                          {t("Queue test event")}
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          disabled={busy || !canEdit || !cfg.webhook_configured}
                          onClick={() => void webhookAction("retry")}
                        >
                          {t("Retry failed")}
                        </Button>
                      </div>
                      {session?.is_superuser && (
                        <>
                          <SMTPSettings
                            value={cfg}
                            onChange={set}
                            password={smtpPassword}
                            onPassword={setSmtpPassword}
                            clear={clearSMTP}
                            onClear={setClearSMTP}
                          />
                          <NotificationEndpoints />
                        </>
                      )}
                      <Actions busy={busy} />
                    </CardContent>
                  </Card>
                )}
                {tab === "system" && (
                  <div className="space-y-4">
                    <DatePreferences />
                    {session?.is_superuser && <PanelUpdate />}
                    <Card>
                      <CardHeader>
                        <div>
                          <CardTitle>{t("Host health")}</CardTitle>
                        </div>
                      </CardHeader>
                      <CardContent className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                        <Read
                          label={t("CPU")}
                          value={
                            Number.isFinite(sys?.cpu_percent)
                              ? tx(
                                  "{0}% · {1} cores",
                                  sys!.cpu_percent.toFixed(1),
                                  sys!.cpu_count ?? "—",
                                )
                              : "—"
                          }
                        />
                        <Read
                          label={t("Memory")}
                          value={
                            Number.isFinite(sys?.memory_percent)
                              ? `${fmt(sys!.memory_used_bytes)} / ${fmt(sys!.memory_total_bytes)} (${sys!.memory_percent.toFixed(1)}%)`
                              : "—"
                          }
                        />
                        <Read
                          label={t("Swap")}
                          value={
                            Number.isFinite(sys?.swap_percent)
                              ? `${fmt(sys!.swap_used_bytes)} / ${fmt(sys!.swap_total_bytes)} (${sys!.swap_percent.toFixed(1)}%)`
                              : "—"
                          }
                        />
                        <Read
                          label={t("Disk")}
                          value={
                            sys
                              ? `${fmt(sys.disk_used_bytes)} / ${fmt(sys.disk_total_bytes)} (${diskPct}%)`
                              : "—"
                          }
                        />
                        <Read
                          label={t("Operating system")}
                          value={sys?.platform || "—"}
                        />
                        <Read label={t("Go")} value={sys?.go_version || "—"} />
                        <Read
                          label={t("Load average")}
                          value={
                            sys?.load_average?.length
                              ? sys.load_average
                                  .map((x) => x.toFixed(2))
                                  .join(" · ")
                              : "—"
                          }
                        />
                        <Read
                          label={t("Admins")}
                          value={String(sys?.admins_total ?? 0)}
                        />
                      </CardContent>
                    </Card>
                    <Card>
                      <CardHeader>
                        <CardTitle>{t("Server management")}</CardTitle>
                      </CardHeader>
                      <CardContent>
                        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
                          {[
                            t("veloray status"),
                            t("veloray doctor"),
                            t("veloray backup"),
                            t("veloray port PORT"),
                            t("veloray logs web"),
                            t("veloray logs agent"),
                            t("veloray ssl EMAIL"),
                            t("veloray update"),
                          ].map((x) => (
                            <div
                              key={x}
                              className="rounded-lg border border-[var(--border)] bg-[var(--surface-2)] px-3 py-2 mono text-[10px] text-[var(--muted-strong)]"
                            >
                              {x}
                            </div>
                          ))}
                        </div>
                      </CardContent>
                    </Card>
                  </div>
                )}
              </fieldset>
            </form>
          </div>
        )}
      </div>
    </div>
  );
}
function Field({
  label,
  children,
  className = "",
}: {
  label: string;
  children: any;
  className?: string;
}) {
  useLocale();
  return (
    <label className={`label ${className}`}>
      {label}
      {children}
    </label>
  );
}
function Info({
  icon,
  label,
  value,
}: {
  icon: string;
  label: string;
  value: string;
}) {
  useLocale();
  return (
    <div className="settings-status-item">
      <FaIcon icon={icon} />
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
function Toggle({
  label,
  text,
  checked,
  onChange,
}: {
  label: string;
  text: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  useLocale();
  return (
    <label className="group flex cursor-pointer items-center justify-between gap-4 rounded-xl border border-[var(--border)] bg-[var(--surface-2)] p-3.5 transition hover:border-[var(--border-strong)]">
      <div>
        <div className="text-[13px] font-semibold">{label}</div>
        <span className="sr-only">{text}</span>
      </div>
      <span
        className={`relative h-6 w-11 shrink-0 rounded-full border transition ${checked ? "border-[var(--brand)] bg-[var(--brand)]" : "border-[var(--border-strong)] bg-[var(--surface-3)]"}`}
      >
        <Input
          className="sr-only"
          type="checkbox"
          aria-label={label}
          checked={checked}
          onChange={(e) => onChange(e.target.checked)}
        />
        <span
          className={`absolute left-0.5 top-0.5 h-5 w-5 rounded-full bg-white shadow-sm transition-transform ${checked ? "translate-x-5" : "translate-x-0"}`}
        />
      </span>
    </label>
  );
}
function Read({ label, value }: { label: string; value: string }) {
  useLocale();
  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--surface-2)] p-3">
      <div className="text-[9px] text-[var(--muted)]">{label}</div>
      <div className="mt-1 break-words text-[11px] font-medium">{value}</div>
    </div>
  );
}
function Actions({ busy }: { busy: boolean }) {
  useLocale();
  return (
    <div className="settings-save-bar sm:col-span-2">
      <span>
        {t(
          "Changes apply after saving.",
          "تغییرات فرم با ذخیره اعمال می‌شوند.",
        )}
      </span>
      <Button type="submit" disabled={busy}>
        {busy ? t("Saving…") : t("Save settings")}
      </Button>
    </div>
  );
}
