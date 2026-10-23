import { useEffect, useState, type FormEvent } from "react";
import { Fingerprint, Plus, Trash2 } from "lucide-react";
import { api } from "@/lib/api";
import { t } from "@/lib/i18n";
import {
  passkeyError,
  registerPasskey,
  supportsPasskeys,
} from "@/lib/passkeys";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/controls";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog } from "@/components/common/dialog";

type Key = {
  id: string;
  name: string;
  created_at: string;
  last_used_at?: string;
};
type Status = { enabled: boolean; credentials: Key[] };
export function PasskeySettings() {
  const [status, setStatus] = useState<Status | null>(null),
    [name, setName] = useState(""),
    [password, setPassword] = useState(""),
    [code, setCode] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState(""),
    [removing, setRemoving] = useState<Key | null>(null);
  const capable = supportsPasskeys();
  async function load() {
    const data = await api<Status>("/auth/passkeys");
    if (!Array.isArray(data.credentials))
      throw new Error(
        t("Passkey status unavailable", "وضعیت کلیدها در دسترس نیست"),
      );
    setStatus(data);
  }
  useEffect(() => {
    const c = new AbortController();
    void api<Status>("/auth/passkeys", { signal: c.signal })
      .then((d) => {
        if (!Array.isArray(d.credentials))
          throw new Error(
            t("Passkey status unavailable", "وضعیت کلیدها در دسترس نیست"),
          );
        setStatus(d);
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(passkeyError(e));
      });
    return () => c.abort();
  }, []);
  async function add(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await registerPasskey(name.trim(), password, code.trim());
      setName("");
      setPassword("");
      setCode("");
      await load();
      window.dispatchEvent(new Event("veloray:session-update"));
      setMessage(t("Passkey added", "کلید ورود اضافه شد"));
    } catch (e) {
      setError(passkeyError(e));
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (!removing || busy) return;
    setBusy(true);
    setError("");
    try {
      await api(`/auth/passkeys/${encodeURIComponent(removing.id)}`, {
        method: "DELETE",
        body: JSON.stringify({ password, code: code.trim() }),
      });
      setRemoving(null);
      setPassword("");
      setCode("");
      await load();
      window.dispatchEvent(new Event("veloray:session-update"));
      setMessage(t("Passkey removed", "کلید ورود حذف شد"));
    } catch (e) {
      setError(passkeyError(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="grid h-9 w-9 place-items-center rounded-lg border border-emerald-500/15 bg-emerald-500/8">
            <Fingerprint className="h-4 w-4 text-emerald-500" />
          </div>
          <CardTitle>{t("Passkeys", "کلیدهای ورود")}</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="space-y-5">
        <p className="text-[12px] leading-6 text-[var(--muted)]">
          {t(
            "Sign in with your device PIN, fingerprint or security key. Confirm your local password to add or remove a passkey.",
            "با پین دستگاه، اثر انگشت یا کلید امنیتی وارد شوید. برای اضافه یا حذف کردن کلید، رمز محلی خود را تأیید کنید.",
          )}
        </p>
        {error && !removing && (
          <p role="alert" className="text-[12px] text-red-500">
            {error}
          </p>
        )}
        {message && (
          <p role="status" className="text-[12px] text-emerald-500">
            {message}
          </p>
        )}
        {!capable || (status && !status.enabled) ? (
          <p className="rounded-lg border border-[var(--border)] p-3 text-[12px] text-[var(--muted)]">
            {t(
              "Passkeys need an enabled panel setting, trusted HTTPS domain and a supported browser.",
              "کلید ورود به فعال بودن گزینهٔ پنل، دامنهٔ HTTPS معتبر و مرورگر سازگار نیاز دارد.",
            )}
          </p>
        ) : null}
        <form onSubmit={add} className="grid items-end gap-3 sm:grid-cols-2">
          <label className="label">
            {t("Passkey name", "نام کلید")}
            <Input
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("My phone", "گوشی من")}
              maxLength={80}
              disabled={busy || !capable || !status?.enabled}
            />
          </label>
          <label className="label">
            {t("Local password", "رمز محلی")}
            <Input
              className="input"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={busy}
            />
          </label>
          <label className="label">
            {t(
              "Authenticator code, if enabled",
              "کد احراز هویت، در صورت فعال بودن",
            )}
            <Input
              className="input"
              value={code}
              autoComplete="one-time-code"
              onChange={(e) => setCode(e.target.value)}
              disabled={busy}
            />
          </label>
          <Button
            type="submit"
            disabled={
              busy || !capable || !status?.enabled || !name.trim() || !password
            }
          >
            <Plus className="h-4 w-4" />
            {t(
              busy ? "Please wait…" : "Add passkey",
              busy ? "لطفاً صبر کنید…" : "افزودن کلید ورود",
            )}
          </Button>
        </form>
        {status &&
          (status.credentials.length ? (
            <ul className="divide-y divide-[var(--border)] rounded-lg border border-[var(--border)]">
              {status.credentials.map((key) => (
                <li
                  key={key.id}
                  className="flex items-center justify-between gap-3 p-3"
                >
                  <div className="min-w-0">
                    <p className="truncate text-[13px] font-medium">
                      {key.name}
                    </p>
                    <p className="mt-1 text-[11px] text-[var(--muted)]">
                      {key.last_used_at
                        ? t("Last used", "آخرین استفاده") +
                          ": " +
                          new Date(key.last_used_at).toLocaleString()
                        : t("Not used yet", "هنوز استفاده نشده")}
                    </p>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={busy || !password}
                    onClick={() => {
                      setError("");
                      setRemoving(key);
                    }}
                    aria-label={t("Remove", "حذف") + " " + key.name}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-[12px] text-[var(--muted)]">
              {t("No passkeys registered", "کلیدی ثبت نشده است")}
            </p>
          ))}
        {removing && (
          <Dialog
            label={t("Remove passkey", "حذف کلید ورود")}
            size="sm"
            onClose={() => {
              if (!busy) setRemoving(null);
            }}
          >
            <div className="space-y-4">
              <h2 className="text-[14px] font-semibold">
                {t("Remove passkey", "حذف کلید ورود")}
              </h2>
              <p className="text-[13px]">{removing.name}</p>
              {error && (
                <p role="alert" className="text-[12px] text-red-500">
                  {error}
                </p>
              )}
              <div className="flex justify-end gap-2">
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() => setRemoving(null)}
                >
                  {t("Cancel", "لغو")}
                </Button>
                <Button
                  type="button"
                  disabled={busy}
                  onClick={() => void remove()}
                >
                  {t("Remove", "حذف")}
                </Button>
              </div>
            </div>
          </Dialog>
        )}
      </CardContent>
    </Card>
  );
}
