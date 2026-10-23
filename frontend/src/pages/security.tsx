import { Input } from "@/components/ui/controls";
import { useLocale, t } from "@/lib/i18n";
import { FormEvent, useEffect, useState } from "react";
import {
  Check,
  Copy,
  Database,
  KeyRound,
  LockKeyhole,
  ShieldCheck,
} from "lucide-react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/common/page-header";
import { PasskeySettings } from "@/components/passkey-settings";
type Status = { enabled: boolean; recovery_codes_remaining: number };
export function SecurityPage() {
  useLocale();
  const [status, setStatus] = useState<Status | null>(null),
    [setup, setSetup] = useState<any>(null),
    [code, setCode] = useState(""),
    [recovery, setRecovery] = useState<string[]>([]),
    [password, setPassword] = useState(""),
    [disableCode, setDisableCode] = useState(""),
    [msg, setMsg] = useState(""),
    [copied, setCopied] = useState(false);
  const load = () => api<Status>("/auth/2fa/status").then(setStatus);
  useEffect(() => {
    load().catch((e) => setMsg(e.message));
  }, []);
  async function begin() {
    try {
      setSetup(await api("/auth/2fa/setup", { method: "POST" }));
      setMsg(t("Add the secret to your authenticator, then confirm a code."));
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function confirm(e: FormEvent) {
    e.preventDefault();
    try {
      const r: any = await api("/auth/2fa/confirm", {
        method: "POST",
        body: JSON.stringify({ code }),
      });
      setRecovery(r.recovery_codes || []);
      setSetup(null);
      setCode("");
      await load();
      window.dispatchEvent(new Event("veloray:session-update"));
      setMsg(
        t("Two-factor authentication enabled. Save the recovery codes now."),
      );
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function disable(e: FormEvent) {
    e.preventDefault();
    try {
      await api("/auth/2fa/disable", {
        method: "POST",
        body: JSON.stringify({ password, code: disableCode }),
      });
      setPassword("");
      setDisableCode("");
      setRecovery([]);
      await load();
      window.dispatchEvent(new Event("veloray:session-update"));
      setMsg(t("Two-factor authentication disabled."));
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  async function copy() {
    await navigator.clipboard.writeText(recovery.join("\n"));
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  }
  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Security")}
        description={t("Protect your account with two-factor authentication.")}
      />
      {msg && (
        <div
          role="status"
          aria-live="polite"
          className="rounded-lg border border-[var(--border)] bg-[var(--card)] px-3 py-2.5 text-[12px] text-[var(--muted-strong)]"
        >
          {msg}
        </div>
      )}
      <div className="grid gap-4 xl:grid-cols-[1.2fr_.8fr]">
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <div className="flex items-center gap-3">
                <div className="grid h-9 w-9 place-items-center rounded-lg border border-emerald-500/15 bg-emerald-500/8">
                  <ShieldCheck className="h-4 w-4 text-emerald-500" />
                </div>
                <div>
                  <CardTitle>{t("Two-factor authentication")}</CardTitle>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {status?.enabled ? (
                <div className="space-y-4">
                  <div className="rounded-lg border border-emerald-500/15 bg-emerald-500/8 p-3 text-[12px] text-emerald-500">
                    {t("Enabled ·")} {status.recovery_codes_remaining}{" "}
                    {t("recovery codes remaining")}
                  </div>
                  <form
                    onSubmit={disable}
                    className="grid gap-3 md:grid-cols-[1fr_1fr_auto]"
                  >
                    <Input
                      className="input"
                      type="password"
                      placeholder={t("Current password")}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      required
                    />
                    <Input
                      className="input"
                      placeholder={t("Authenticator / recovery code")}
                      value={disableCode}
                      onChange={(e) => setDisableCode(e.target.value)}
                      required
                    />
                    <Button type="submit" variant="outline">
                      {t("Disable 2FA")}
                    </Button>
                  </form>
                </div>
              ) : setup ? (
                <form onSubmit={confirm} className="space-y-4">
                  <div className="rounded-lg border border-[var(--border)] bg-[var(--surface-2)] p-4">
                    <div className="text-[10px] uppercase tracking-wider text-[var(--muted)]">
                      {t("Authenticator secret")}
                    </div>
                    <div className="mt-2 break-all mono text-[13px]">
                      {setup.secret}
                    </div>
                    <div className="mt-3 break-all text-[9px] leading-4 text-[var(--muted)]">
                      {setup.otpauth_uri}
                    </div>
                  </div>
                  <div className="flex flex-col gap-2 sm:flex-row">
                    <Input
                      className="input"
                      placeholder={t("6-digit code")}
                      value={code}
                      onChange={(e) => setCode(e.target.value)}
                      required
                    />
                    <Button type="submit">{t("Confirm & enable")}</Button>
                  </div>
                </form>
              ) : (
                <Button onClick={begin}>
                  <KeyRound className="h-3.5 w-3.5" />
                  {t("Set up 2FA")}
                </Button>
              )}
              {recovery.length > 0 && (
                <div className="mt-5 rounded-lg border border-amber-500/15 bg-amber-500/8 p-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <div className="text-[12px] font-semibold text-amber-500">
                        {t("Recovery codes")}
                      </div>
                      <div className="mt-1 text-[10px] text-[var(--muted)]">
                        {t("Store these offline. Each code works once.")}
                      </div>
                    </div>
                    <Button size="sm" variant="outline" onClick={copy}>
                      {copied ? (
                        <Check className="h-3.5 w-3.5" />
                      ) : (
                        <Copy className="h-3.5 w-3.5" />
                      )}
                      {t("Copy")}
                    </Button>
                  </div>
                  <div className="mt-3 grid grid-cols-2 gap-2 mono text-[11px] sm:grid-cols-5">
                    {recovery.map((x) => (
                      <span key={x}>{x}</span>
                    ))}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
          <PasskeySettings />
        </div>
        <div className="grid gap-3 content-start sm:grid-cols-3 xl:grid-cols-1">
          {[
            [
              KeyRound,
              t("Node credentials"),
              t("Agent bearer tokens are encrypted at rest."),
            ],
            [
              LockKeyhole,
              t("Agent transport"),
              t("Local Agent stays loopback-only; remote Agents require TLS."),
            ],
            [
              Database,
              t("Recovery"),
              t(
                "Backup, restore and admin recovery remain available through the host CLI.",
              ),
            ],
          ].map(([I, a, b]: any) => (
            <Card key={a}>
              <CardContent className="flex gap-3 p-4">
                <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg border border-[var(--border)] bg-[var(--surface-2)]">
                  <I className="h-4 w-4 text-[var(--muted-strong)]" />
                </div>
                <div>
                  <div className="text-[12px] font-semibold">{a}</div>
                  <p className="mt-1 text-[10px] leading-5 text-[var(--muted)]">
                    {b}
                  </p>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    </div>
  );
}
