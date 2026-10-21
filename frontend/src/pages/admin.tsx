import { formatDate } from "@/lib/dates";
import { Select, Input } from "@/components/ui/controls";
import { t } from "@/lib/i18n";
import { useLocale, tx } from "@/lib/i18n";
const permissionOptions = [
  ["dashboard.read", "View dashboard"],
  ["clients.read", "View clients"],
  ["clients.write", "Create and edit clients"],
  ["clients.delete", "Delete clients"],
  ["inbounds.read", "View inbounds"],
  ["inbounds.write", "Manage inbounds"],
  ["nodes.read", "View nodes"],
  ["nodes.write", "Manage nodes"],
  ["traffic.read", "View traffic"],
  ["xray.read", "View Xray"],
  ["xray.write", "Manage Xray"],
  ["settings.read", "View settings"],
  ["api.read", "View API reference"],
];
import { Dialog } from "@/components/common/dialog";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { Copy, KeyRound, Plus, Search, Trash2, UserCog, X } from "lucide-react";
import {
  api,
  type AdminAPIKey,
  type AdminUser,
  type InboundItem,
  type AppSettings,
  type SystemInfo,
} from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/common/page-header";
import { FaIcon } from "@/components/ui/fa-icon";

type Editor = {
  ldap_dn: string;
  oidc_issuer: string;
  oidc_subject: string;
  id?: number;
  username: string;
  email: string;
  password: string;
  is_active: boolean;
  is_staff: boolean;
  is_superuser: boolean;
  role: "administrator" | "reseller" | "viewer" | "custom";
  permissions: string[];
  allowed_inbounds: number[];
  own_clients_only: boolean;
  max_clients: string;
  budget: string;
};
type KeyEditor = {
  name: string;
  scope: "read" | "write" | "admin";
  user: string;
  days: string;
};
const blank = (): Editor => ({
  ldap_dn: "",
  oidc_issuer: "",
  oidc_subject: "",
  username: "",
  email: "",
  password: "",
  is_active: true,
  is_staff: true,
  is_superuser: false,
  role: "reseller",
  permissions: [],
  allowed_inbounds: [],
  own_clients_only: true,
  max_clients: "0",
  budget: "0",
});

export function AdminPage() {
  useLocale();
  const [inbounds, setInbounds] = useState<InboundItem[]>([]);
  const [users, setUsers] = useState<AdminUser[]>([]),
    [keys, setKeys] = useState<AdminAPIKey[]>([]),
    [sys, setSys] = useState<SystemInfo | null>(null),
    [cfg, setCfg] = useState<AppSettings | null>(null);
  const [query, setQuery] = useState(""),
    [editor, setEditor] = useState<Editor | null>(null),
    [keyEditor, setKeyEditor] = useState<KeyEditor | null>(null),
    [createdToken, setCreatedToken] = useState("");
  const [tab, setTab] = useState<"operators" | "api">("operators"),
    [msg, setMsg] = useState(""),
    [busy, setBusy] = useState(false);
  async function load() {
    try {
      const [u, k, s, c, i] = await Promise.all([
        api<AdminUser[]>("/admin/users/"),
        api<AdminAPIKey[]>("/admin/api-keys/"),
        api<SystemInfo>("/system-info"),
        api<AppSettings>("/settings"),
        api<InboundItem[]>("/inbounds/"),
      ]);
      setUsers(u);
      setInbounds(i);
      setKeys(k);
      setSys(s);
      setCfg(c);
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  useEffect(() => {
    load();
  }, []);
  const filtered = useMemo(() => {
    const q = query.toLowerCase().trim();
    return q
      ? users.filter((u) =>
          `${u.username} ${u.email}`.toLowerCase().includes(q),
        )
      : users;
  }, [users, query]);
  function edit(u: AdminUser) {
    setEditor({
      id: u.id,
      username: u.username,
      email: u.email || "",
      ldap_dn: u.ldap_dn || "",
      oidc_issuer: u.oidc_issuer || "",
      oidc_subject: u.oidc_subject || "",
      password: "",
      is_active: u.is_active,
      is_staff: u.is_staff,
      is_superuser: u.is_superuser,
      role: u.role || (u.is_superuser ? "administrator" : "custom"),
      permissions: u.permissions || [],
      allowed_inbounds: u.allowed_inbounds || [],
      own_clients_only: !!u.own_clients_only,
      max_clients: String(u.max_clients || 0),
      budget: String((u.traffic_budget_bytes || 0) / 1024 ** 3),
    });
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!editor) return;
    setBusy(true);
    setMsg("");
    try {
      const body: any = {
        username: editor.username,
        email: editor.email,
        is_active: editor.is_active,
        is_staff: editor.is_staff,
        role: editor.role,
        permissions: editor.permissions,
        allowed_inbounds: editor.allowed_inbounds,
        own_clients_only: editor.role === "reseller" || editor.own_clients_only,
        max_clients: Number(editor.max_clients),
        traffic_budget_bytes: Math.round(Number(editor.budget) * 1024 ** 3),
      };
      body.ldap_dn = editor.ldap_dn;
      body.oidc_issuer = editor.oidc_issuer;
      body.oidc_subject = editor.oidc_subject;
      if (editor.password) body.password = editor.password;
      if (editor.id)
        await api(`/admin/users/${editor.id}/`, {
          method: "PATCH",
          body: JSON.stringify(body),
        });
      else {
        if (!editor.password)
          throw new Error(t("Password is required for a new administrator."));
        await api("/admin/users/", {
          method: "POST",
          body: JSON.stringify(body),
        });
      }
      setEditor(null);
      setMsg(t("Account saved."));
      await load();
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function remove(u: AdminUser) {
    if (!confirm(tx("Delete administrator {0}?", u.username))) return;
    try {
      await api(`/admin/users/${u.id}/`, { method: "DELETE" });
      await load();
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function createKey(e: FormEvent) {
    e.preventDefault();
    if (!keyEditor) return;
    setBusy(true);
    setCreatedToken("");
    try {
      const expires = new Date(
        Date.now() + Math.max(1, Number(keyEditor.days) || 90) * 86400000,
      ).toISOString();
      const out = await api<AdminAPIKey>("/admin/api-keys/", {
        method: "POST",
        body: JSON.stringify({
          name: keyEditor.name,
          scope: keyEditor.scope,
          user: Number(keyEditor.user),
          expires_at: expires,
        }),
      });
      setCreatedToken(out.token || "");
      setKeyEditor(null);
      setMsg(
        t("API key created. Copy it now; the full token is shown only once."),
      );
      await load();
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(false);
    }
  }
  async function revokeKey(k: AdminAPIKey) {
    if (!confirm(tx("Revoke API key {0}?", k.name))) return;
    try {
      await api(`/admin/api-keys/${k.id}/`, { method: "DELETE" });
      await load();
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function toggleKey(k: AdminAPIKey) {
    try {
      await api(`/admin/api-keys/${k.id}/`, {
        method: "PATCH",
        body: JSON.stringify({ enabled: !k.enabled }),
      });
      await load();
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function copyToken() {
    if (!createdToken) return;
    await navigator.clipboard.writeText(createdToken);
    setMsg(t("API key copied."));
  }
  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Accounts")}
        description={t("Operators, access policy and scoped API credentials.")}
        actions={
          <Button
            onClick={() =>
              tab === "operators"
                ? setEditor(blank())
                : setKeyEditor({
                    name: t("Automation"),
                    scope: "read",
                    user: String(users[0]?.id || ""),
                    days: String(cfg?.api_key_default_days || 90),
                  })
            }
          >
            <Plus className="h-3.5 w-3.5" />
            {tab === "operators" ? t("Add account") : t("Create API key")}
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
      {createdToken && (
        <Card className="border-emerald-500/30">
          <CardHeader>
            <div>
              <CardTitle>{t("New API key")}</CardTitle>
            </div>
            <Badge tone="green">{t("One-time secret")}</Badge>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-2 sm:flex-row">
              <code className="min-w-0 flex-1 break-all rounded-lg border border-[var(--border)] bg-[var(--surface-2)] p-3 text-[10px]">
                {createdToken}
              </code>
              <Button variant="outline" onClick={copyToken}>
                <Copy className="h-3.5 w-3.5" />
                {t("Copy")}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      {cfg?.require_2fa_for_admins &&
        users.some((u) => u.is_staff && !u.two_factor_enabled) && (
          <div className="rounded-lg border border-amber-500/20 bg-amber-500/8 px-4 py-3 text-[11px] text-amber-500">
            <div className="font-semibold">
              {t("2FA policy needs attention")}
            </div>
            <div className="mt-1 text-[10px] opacity-90">
              {users.filter((u) => u.is_staff && !u.two_factor_enabled).length}{" "}
              {t("administrator account(s) do not have TOTP enabled yet.")}
            </div>
          </div>
        )}
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Mini
          icon="users"
          label={t("Panel users")}
          value={String(sys?.users_total ?? users.length)}
        />
        <Mini
          icon="user-shield"
          label={t("Administrators")}
          value={String(
            sys?.admins_total ?? users.filter((x) => x.is_staff).length,
          )}
        />
        <Mini
          icon="key"
          label={t("Active API keys")}
          value={String(keys.filter((x) => x.enabled).length)}
        />
        <Mini
          icon="shield-halved"
          label={t("2FA enabled")}
          value={String(users.filter((x) => x.two_factor_enabled).length)}
        />
      </section>
      <div className="flex gap-1 rounded-lg border border-[var(--border)] bg-[var(--card)] p-1">
        {[
          ["operators", t("Accounts")],
          ["api", t("API keys")],
        ].map(([k, l]) => (
          <button
            key={k}
            onClick={() => setTab(k as any)}
            className={`rounded-md px-3 py-2 text-[11px] font-semibold ${tab === k ? "bg-[var(--surface-2)] text-[var(--fg)]" : "text-[var(--muted)]"}`}
          >
            {l}
          </button>
        ))}
      </div>
      {tab === "operators" ? (
        <Card>
          <CardHeader>
            <div>
              <CardTitle>{t("Account directory")}</CardTitle>
            </div>
            <div className="relative w-full max-w-64">
              <Search className="pointer-events-none absolute left-3 top-2.5 h-3.5 w-3.5 text-[var(--muted)]" />
              <Input
                className="input h-9 pl-8"
                placeholder={t("Search users")}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
          </CardHeader>
          <CardContent className="p-0">
            <div className="table-wrap">
              <table className="data-table min-w-[760px]">
                <thead>
                  <tr>
                    <th>{t("User")}</th>
                    <th>{t("Role")}</th>
                    <th>{t("2FA")}</th>
                    <th>{t("Status")}</th>
                    <th>{t("Last login")}</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((u) => (
                    <tr key={u.id}>
                      <td>
                        <div className="font-semibold text-[var(--fg)]">
                          {u.username}
                        </div>
                        <div className="mt-0.5 text-[9px] text-[var(--muted)]">
                          {u.email || t("No email")}
                          <div className="mt-1 text-[10px] text-[var(--muted)]">
                            {u.clients_count || 0} {t("clients")} ·{" "}
                            {(
                              (u.allocated_traffic_bytes || 0) /
                              1024 ** 3
                            ).toFixed(1)}{" "}
                            {t("GB")}
                          </div>
                        </div>
                      </td>
                      <td>
                        {u.role === "reseller" ? (
                          <Badge tone="blue">{t("Reseller")}</Badge>
                        ) : u.role === "viewer" ? (
                          <Badge>{t("Viewer")}</Badge>
                        ) : u.is_superuser ? (
                          <Badge tone="blue">{t("Superuser")}</Badge>
                        ) : u.is_staff ? (
                          <Badge tone="neutral">{t("Staff")}</Badge>
                        ) : (
                          <Badge tone="amber">{t("Limited")}</Badge>
                        )}
                      </td>
                      <td>
                        <Badge tone={u.two_factor_enabled ? "green" : "amber"}>
                          {u.two_factor_enabled ? t("Enabled") : t("Off")}
                        </Badge>
                      </td>
                      <td>
                        <Badge tone={u.is_active ? "green" : "red"}>
                          {u.is_active ? t("Active") : t("Disabled")}
                        </Badge>
                      </td>
                      <td>
                        {u.last_login
                          ? formatDate(u.last_login, {
                              hour: "2-digit",
                              minute: "2-digit",
                            })
                          : t("Never")}
                      </td>
                      <td>
                        <div className="flex justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            title={t("Edit")}
                            onClick={() => edit(u)}
                          >
                            <UserCog className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            title={t("Delete")}
                            onClick={() => remove(u)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!filtered.length && (
              <div className="p-12 text-center text-[11px] text-[var(--muted)]">
                {t("No administrators found.")}
              </div>
            )}
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <div>
              <CardTitle>{t("Scoped API keys")}</CardTitle>
            </div>
            <Badge tone={cfg?.api_keys_enabled ? "green" : "amber"}>
              {cfg?.api_keys_enabled ? t("Enabled") : t("Disabled in Settings")}
            </Badge>
          </CardHeader>
          <CardContent className="p-0">
            <div className="table-wrap">
              <table className="data-table min-w-[760px]">
                <thead>
                  <tr>
                    <th>{t("Name")}</th>
                    <th>{t("Owner")}</th>
                    <th>{t("Prefix")}</th>
                    <th>{t("Scope")}</th>
                    <th>{t("Expires")}</th>
                    <th>{t("Last used")}</th>
                    <th>{t("Status")}</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {keys.map((k) => (
                    <tr key={k.id}>
                      <td className="font-semibold text-[var(--fg)]">
                        {k.name}
                      </td>
                      <td>{k.user_name}</td>
                      <td className="mono">{k.prefix}…</td>
                      <td>
                        <Badge
                          tone={
                            k.scope === "admin"
                              ? "blue"
                              : k.scope === "write"
                                ? "green"
                                : "neutral"
                          }
                        >
                          {k.scope}
                        </Badge>
                      </td>
                      <td>
                        {k.expires_at ? formatDate(k.expires_at) : t("Never")}
                      </td>
                      <td>
                        {k.last_used_at
                          ? formatDate(k.last_used_at, {
                              hour: "2-digit",
                              minute: "2-digit",
                            })
                          : t("Never")}
                      </td>
                      <td>
                        <Badge tone={k.enabled ? "green" : "amber"}>
                          {k.enabled ? t("Active") : t("Paused")}
                        </Badge>
                      </td>
                      <td>
                        <div className="flex justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => toggleKey(k)}
                          >
                            {k.enabled ? t("Pause") : t("Enable")}
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            title={t("Revoke")}
                            onClick={() => revokeKey(k)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!keys.length && (
              <div className="p-12 text-center text-[11px] text-[var(--muted)]">
                {t("No API keys created.")}
              </div>
            )}
          </CardContent>
        </Card>
      )}
      {editor && (
        <Dialog
          padded={false}
          label={t("Administrator")}
          onClose={() => {
            if (!busy) setEditor(null);
          }}
        >
          <div className="flex items-start justify-between border-b border-[var(--border)] px-5 py-4">
            <div>
              <div className="text-[14px] font-semibold">
                {editor.id ? t("Edit account") : t("Add account")}
              </div>
              <div className="mt-1 text-[10px] text-[var(--muted)]">
                {t("Use strong passwords and enable 2FA after first login.")}
              </div>
            </div>
            <Button variant="ghost" size="icon" onClick={() => setEditor(null)}>
              <X className="h-4 w-4" />
            </Button>
          </div>
          <form onSubmit={save}>
            <div className="grid gap-4 p-5 sm:grid-cols-2">
              <Field label={t("Username")}>
                <Input
                  className="input"
                  value={editor.username}
                  onChange={(e) =>
                    setEditor({ ...editor, username: e.target.value })
                  }
                  required
                />
              </Field>
              <Field label={t("Email")}>
                <Input
                  className="input"
                  type="email"
                  value={editor.email}
                  onChange={(e) =>
                    setEditor({ ...editor, email: e.target.value })
                  }
                />
              </Field>
              <Field
                label={t("LDAP distinguished name (optional)")}
                className="sm:col-span-2"
              >
                <Input
                  value={editor.ldap_dn}
                  aria-label={t("LDAP distinguished name (optional)")}
                  aria-describedby="ldap-binding-hint"
                  maxLength={1024}
                  autoComplete="off"
                  placeholder="uid=alice,ou=people,dc=example,dc=com"
                  onChange={(e) =>
                    setEditor({ ...editor, ldap_dn: e.target.value })
                  }
                />
                <p
                  id="ldap-binding-hint"
                  className="field-hint text-xs text-[var(--muted)]"
                >
                  {t(
                    "Requires LDAPS in server configuration. Sign in with the panel username and directory password; local permissions and 2FA still apply.",
                  )}
                </p>
              </Field>
              <Field label={t("OIDC issuer (optional)")}>
                <Input
                  type="url"
                  value={editor.oidc_issuer}
                  onChange={(e) =>
                    setEditor({ ...editor, oidc_issuer: e.target.value })
                  }
                  placeholder="https://id.example/realm"
                />
              </Field>
              <Field label={t("OIDC subject (optional)")}>
                <Input
                  value={editor.oidc_subject}
                  maxLength={512}
                  onChange={(e) =>
                    setEditor({ ...editor, oidc_subject: e.target.value })
                  }
                />
              </Field>
              <p className="text-xs text-[var(--muted)] sm:col-span-2">
                {t(
                  "For GitHub, use issuer https://github.com and the numeric account ID. For Telegram, use https://oauth.telegram.org and the verified OIDC subject. Each user has one SSO binding.",
                )}
              </p>
              <Field
                label={editor.id ? t("New password (optional)") : t("Password")}
                className="sm:col-span-2"
              >
                <Input
                  className="input"
                  type="password"
                  minLength={12}
                  value={editor.password}
                  onChange={(e) =>
                    setEditor({ ...editor, password: e.target.value })
                  }
                  required={!editor.id}
                />
              </Field>
              <Toggle
                label={t("Active account")}
                checked={editor.is_active}
                onChange={(v) => setEditor({ ...editor, is_active: v })}
              />
              <Field label={t("Role")}>
                <Select
                  className="input"
                  aria-label={t("Role")}
                  value={editor.role}
                  onChange={(e) =>
                    setEditor({
                      ...editor,
                      role: e.target.value as Editor["role"],
                    })
                  }
                >
                  <option value="administrator">{t("Administrator")}</option>
                  <option value="reseller">{t("Reseller")}</option>
                  <option value="viewer">{t("Viewer")}</option>
                  <option value="custom">{t("Custom access")}</option>
                </Select>
              </Field>
              {editor.role !== "administrator" && (
                <>
                  <Toggle
                    label={t("Own clients only")}
                    checked={
                      editor.role === "reseller" || editor.own_clients_only
                    }
                    onChange={(v) =>
                      setEditor({ ...editor, own_clients_only: v })
                    }
                  />
                  <Field label={t("Client limit")}>
                    <Input
                      className="input"
                      type="number"
                      min="0"
                      value={editor.max_clients}
                      onChange={(e) =>
                        setEditor({ ...editor, max_clients: e.target.value })
                      }
                    />
                  </Field>
                  <Field label={t("Traffic budget (GB)")}>
                    <Input
                      className="input"
                      type="number"
                      min="0"
                      step=".1"
                      value={editor.budget}
                      onChange={(e) =>
                        setEditor({ ...editor, budget: e.target.value })
                      }
                    />
                  </Field>
                  <fieldset className="sm:col-span-2">
                    <legend className="label mb-2">
                      {t("Assigned inbounds")}
                    </legend>
                    <div className="grid gap-2 sm:grid-cols-2">
                      {inbounds.map((i) => (
                        <label
                          key={i.id}
                          className="flex items-center gap-2 text-xs"
                        >
                          <Input
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
                          <span>
                            {i.name} · {i.node_name}
                          </span>
                        </label>
                      ))}
                    </div>
                  </fieldset>
                  {editor.role === "reseller" &&
                    !editor.allowed_inbounds.length && (
                      <p className="sm:col-span-2 text-xs text-amber-500">
                        {t(
                          "Assign an inbound before this reseller can create clients.",
                        )}
                      </p>
                    )}
                  {editor.role === "custom" && (
                    <fieldset className="sm:col-span-2">
                      <legend className="label mb-2">{t("Permissions")}</legend>
                      <div className="grid gap-2 sm:grid-cols-2">
                        {permissionOptions.map(([key, label]) => (
                          <label
                            key={key}
                            className="flex items-center gap-2 text-xs"
                          >
                            <Input
                              type="checkbox"
                              checked={editor.permissions.includes(key)}
                              onChange={(e) =>
                                setEditor({
                                  ...editor,
                                  permissions: e.target.checked
                                    ? [...editor.permissions, key]
                                    : editor.permissions.filter(
                                        (p) => p !== key,
                                      ),
                                })
                              }
                            />
                            {t(label)}
                          </label>
                        ))}
                      </div>
                    </fieldset>
                  )}
                </>
              )}
            </div>
            <div className="flex justify-end gap-2 border-t border-[var(--border)] px-5 py-4">
              <Button variant="outline" onClick={() => setEditor(null)}>
                {t("Cancel")}
              </Button>
              <Button type="submit" disabled={busy}>
                {busy ? "Saving…" : t("Save account")}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
      {keyEditor && (
        <Dialog
          padded={false}
          label={t("Create API key")}
          onClose={() => {
            if (!busy) setKeyEditor(null);
          }}
        >
          <div className="flex items-start justify-between border-b border-[var(--border)] px-5 py-4">
            <div>
              <div className="text-[14px] font-semibold">
                {t("Create API key")}
              </div>
              <div className="mt-1 text-[10px] text-[var(--muted)]">
                {t("The full key is displayed only once after creation.")}
              </div>
            </div>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setKeyEditor(null)}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
          <form onSubmit={createKey}>
            <div className="grid gap-4 p-5 sm:grid-cols-2">
              <Field label={t("Key name")}>
                <Input
                  className="input"
                  value={keyEditor.name}
                  onChange={(e) =>
                    setKeyEditor({ ...keyEditor, name: e.target.value })
                  }
                  required
                />
              </Field>
              <Field label={t("Owner")}>
                <Select
                  className="input"
                  value={keyEditor.user}
                  onChange={(e) =>
                    setKeyEditor({ ...keyEditor, user: e.target.value })
                  }
                >
                  {users
                    .filter((u) => u.is_staff && u.is_active)
                    .map((u) => (
                      <option key={u.id} value={u.id}>
                        {u.username}
                      </option>
                    ))}
                </Select>
              </Field>
              <Field label={t("Scope")}>
                <Select
                  className="input"
                  value={keyEditor.scope}
                  onChange={(e) =>
                    setKeyEditor({ ...keyEditor, scope: e.target.value as any })
                  }
                >
                  <option value="read">{t("Read only")}</option>
                  <option value="write">{t("Read + write")}</option>
                  <option value="admin">{t("Administrator")}</option>
                </Select>
              </Field>
              <Field label={t("Expires in days")}>
                <Input
                  className="input"
                  type="number"
                  min="1"
                  max="3650"
                  value={keyEditor.days}
                  onChange={(e) =>
                    setKeyEditor({ ...keyEditor, days: e.target.value })
                  }
                />
              </Field>
            </div>
            <div className="flex justify-end gap-2 border-t border-[var(--border)] px-5 py-4">
              <Button variant="outline" onClick={() => setKeyEditor(null)}>
                {t("Cancel")}
              </Button>
              <Button type="submit" disabled={busy}>
                <KeyRound className="h-3.5 w-3.5" />
                {busy ? "Creating…" : t("Create key")}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
    </div>
  );
}
function Mini({
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
    <Card>
      <CardContent className="flex items-center justify-between p-4">
        <div>
          <div className="text-[9px] text-[var(--muted)]">{label}</div>
          <div className="mt-1.5 text-[22px] font-semibold">{value}</div>
        </div>
        <div className="grid h-9 w-9 place-items-center rounded-lg bg-[var(--surface-2)]">
          <FaIcon icon={icon} />
        </div>
      </CardContent>
    </Card>
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
function Toggle({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  useLocale();
  return (
    <label className="flex items-center justify-between rounded-lg border border-[var(--border)] bg-[var(--surface-2)] px-3 py-2.5 text-[11px]">
      <span>{label}</span>
      <Input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
    </label>
  );
}
