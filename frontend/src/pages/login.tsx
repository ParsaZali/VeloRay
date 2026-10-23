import { Input, Select } from "@/components/ui/controls";
import { useLocale, type Locale } from "@/lib/i18n";
import { FormEvent, useEffect, useMemo, useState } from "react";
import {
  ArrowLeft,
  Eye,
  EyeOff,
  Fingerprint,
  KeyRound,
  LoaderCircle,
  LogIn,
  Moon,
  ShieldCheck,
  Sun,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { VeloRayMark } from "@/components/brand/veloray-mark";
import { api, login } from "@/lib/api";
import { useTheme } from "@/components/theme/theme-provider";
import {
  passkeyError,
  signInWithPasskey,
  supportsPasskeys,
} from "@/lib/passkeys";

const copy = {
  en: {
    title: "Login to your account",
    username: "Username",
    password: "Password",
    code: "Authenticator / recovery code",
    signIn: "Login",
    verify: "Verify and continue",
    back: "Back to login",
    loading: "Please wait…",
    version: "VeloRay v1.0",
    usernamePlaceholder: "Username",
    passwordPlaceholder: "Password",
    codePlaceholder: "000000",
    verifyTitle: "Verify your identity",
    verifySubtitle:
      "Enter the code from your authenticator app or use a recovery code.",
  },
  fa: {
    title: "ورود به حساب کاربری",
    username: "نام کاربری",
    password: "رمز عبور",
    code: "کد احراز هویت / بازیابی",
    signIn: "ورود",
    verify: "تأیید و ادامه",
    back: "بازگشت به ورود",
    loading: "لطفاً صبر کنید…",
    version: "VeloRay v1.0",
    usernamePlaceholder: "نام کاربری",
    passwordPlaceholder: "رمز عبور",
    codePlaceholder: "000000",
    verifyTitle: "تأیید هویت",
    verifySubtitle:
      "کد برنامه احراز هویت یا یکی از کدهای بازیابی را وارد کنید.",
  },
} as const;

export function LoginPage({ onSuccess }: { onSuccess: () => void }) {
  const [passkeyEnabled, setPasskeyEnabled] = useState(false);
  useEffect(() => {
    if (!supportsPasskeys()) return;
    const c = new AbortController();
    void api<{ enabled: boolean }>("/auth/passkey/status", { signal: c.signal })
      .then((d) => {
        if (!c.signal.aborted) setPasskeyEnabled(d.enabled === true);
      })
      .catch(() => {});
    return () => c.abort();
  }, []);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [oidcChallenge, setOidcChallenge] = useState(
    () => new URLSearchParams(location.search).get("oidc") === "totp",
  );
  const [oidcEnabled, setOidcEnabled] = useState(false);
  const [githubEnabled, setGithubEnabled] = useState(false);
  const [ldapEnabled, setLdapEnabled] = useState(false);
  const [provider, setProvider] = useState<"local" | "ldap">("local");
  const [telegram, setTelegram] = useState(false);
  const [need2fa, setNeed2fa] = useState(oidcChallenge);
  useEffect(() => {
    const c = new AbortController();
    void api<{
      enabled: boolean;
      github_enabled?: boolean;
      ldap_enabled?: boolean;
      telegram?: boolean;
    }>("/auth/oidc", { signal: c.signal })
      .then((d) => {
        if (!c.signal.aborted) {
          setOidcEnabled(d.enabled);
          setGithubEnabled(d.github_enabled === true);
          setLdapEnabled(d.ldap_enabled === true);
          setTelegram(d.telegram === true);
        }
      })
      .catch(() => {});
    return () => c.abort();
  }, []);
  const [error, setError] = useState(() =>
    new URLSearchParams(location.search).get("oidc") === "error"
      ? document.documentElement.lang === "fa"
        ? "ورود سازمانی ناموفق بود؛ دوباره تلاش کنید."
        : "SSO sign-in failed. Restart sign-in."
      : "",
  );
  const [busy, setBusy] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const { locale, setLocale } = useLocale();
  const { resolved, setTheme } = useTheme();

  const t = copy[locale];
  const rtl = locale === "fa";
  const formTitle = need2fa ? t.verifyTitle : t.title;

  const canSubmit = useMemo(() => {
    if (busy) return false;
    return need2fa
      ? Boolean(code.trim())
      : Boolean(username.trim() && password);
  }, [busy, need2fa, code, username, password]);

  function toggleLocale() {
    const next: Locale = locale === "en" ? "fa" : "en";
    localStorage.setItem("veloray-login-locale", next);
    setLocale(next);
    setError("");
  }
  async function startPasskey() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await signInWithPasskey();
      history.replaceState({}, "", "/");
      onSuccess();
    } catch (e) {
      setError(passkeyError(e));
    } finally {
      setBusy(false);
    }
  }

  async function startSSO(provider: "oidc" | "github") {
    setBusy(true);
    setError("");
    try {
      await api("/auth/csrf");
      const data = await api<{ authorization_url: string }>(
        `/auth/${provider}/start`,
        { method: "POST", body: "{}" },
      );
      const url = new URL(data.authorization_url);
      if (
        url.protocol !== "https:" ||
        (provider === "github" &&
          (url.hostname !== "github.com" ||
            url.pathname !== "/login/oauth/authorize"))
      )
        throw Error("Invalid identity provider URL");
      window.location.assign(url.href);
    } catch (error) {
      setError(error instanceof Error ? error.message : "SSO unavailable");
      setBusy(false);
    }
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    setError("");
    setBusy(true);
    try {
      let result: any;
      if (oidcChallenge) {
        await api("/auth/csrf");
        result = await api("/auth/oidc/complete", {
          method: "POST",
          body: JSON.stringify({ totp_code: code.trim() }),
        });
      } else
        result = await login(username.trim(), password, code.trim(), provider);
      if (result.two_factor_required) {
        setNeed2fa(true);
        setCode("");
        return;
      }
      history.replaceState({}, "", "/");
      onSuccess();
    } catch (err: any) {
      setError(err?.message || "Unable to sign in");
    } finally {
      setBusy(false);
    }
  }

  function backToPassword() {
    setOidcChallenge(false);
    history.replaceState({}, "", "/");
    setNeed2fa(false);
    setCode("");
    setError("");
  }

  return (
    <main
      className="login-page min-h-screen bg-[var(--bg)] text-[var(--fg)]"
      lang={locale}
      dir={rtl ? "rtl" : "ltr"}
    >
      <div className="flex min-h-screen w-full flex-col justify-between p-5 sm:p-6">
        <header className="flex w-full items-center justify-between">
          <a href="/" className="login-brand">
            <VeloRayMark />
            VeloRay
          </a>
          <div className="login-tools">
            <button
              type="button"
              onClick={toggleLocale}
              className="inline-flex h-9 min-w-9 items-center justify-center rounded-md border border-[var(--border)] bg-[var(--card)] px-2.5 text-[11px] font-medium text-[var(--muted-strong)] shadow-sm transition hover:bg-[var(--surface-2)] hover:text-[var(--fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ring)]"
              aria-label="Change login language"
            >
              {locale === "en" ? "Persian" : "انگلیسی"}
            </button>

            <button
              type="button"
              onClick={(event) => {
                const rect = event.currentTarget.getBoundingClientRect();
                setTheme(resolved === "dark" ? "light" : "dark", {
                  x: rect.left + rect.width / 2,
                  y: rect.top + rect.height / 2,
                });
              }}
              className="grid h-9 w-9 place-items-center rounded-md border border-[var(--border)] bg-[var(--card)] text-[var(--muted-strong)] shadow-sm transition hover:bg-[var(--surface-2)] hover:text-[var(--fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ring)]"
              aria-label="Toggle theme"
            >
              {resolved === "dark" ? (
                <Sun className="h-4 w-4" />
              ) : (
                <Moon className="h-4 w-4" />
              )}
            </button>
          </div>
        </header>

        <section className="flex w-full flex-1 items-center justify-center py-8 sm:py-12">
          <div className="login-card w-full max-w-[380px]">
            <div className="flex flex-col items-center gap-2 text-center">
              <div className="login-badge">
                <LogIn size={20} />
              </div>
              <h1 className="mt-2 text-2xl font-semibold tracking-[-.025em] text-[var(--fg)]">
                {formTitle}
              </h1>
            </div>

            <div className="mx-auto w-full max-w-[300px] pt-4">
              <form
                onSubmit={submit}
                autoComplete="on"
                className="mt-4 space-y-3"
              >
                {!need2fa ? (
                  <>
                    {ldapEnabled && (
                      <label className="block text-xs text-[var(--muted)]">
                        {rtl ? "روش ورود" : "Sign-in method"}
                        <Select
                          value={provider}
                          onChange={(e) => {
                            setProvider(e.target.value as "local" | "ldap");
                            setPassword("");
                            setError("");
                          }}
                          aria-label={rtl ? "روش ورود" : "Sign-in method"}
                          disabled={busy}
                        >
                          <option value="local">
                            {rtl ? "حساب محلی" : "Local account"}
                          </option>
                          <option value="ldap">
                            {rtl ? "دایرکتوری LDAP" : "LDAP directory"}
                          </option>
                        </Select>
                      </label>
                    )}
                    {provider === "ldap" && (
                      <p className="text-xs text-[var(--muted)]">
                        {rtl
                          ? "نام کاربری پنل و رمز دایرکتوری را وارد کنید."
                          : "Use your panel username and directory password."}
                      </p>
                    )}
                    <label className="block">
                      <span className="sr-only">{t.username}</span>
                      <Input
                        autoFocus
                        autoComplete="username"
                        value={username}
                        onChange={(e) => setUsername(e.target.value)}
                        className="input h-11 bg-[var(--card)] text-[13px]"
                        placeholder={t.usernamePlaceholder}
                        aria-label={t.username}
                      />
                    </label>

                    <label className="relative block">
                      <span className="sr-only">{t.password}</span>
                      <Input
                        type={showPassword ? "text" : "password"}
                        autoComplete="current-password"
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        className={`input h-11 bg-[var(--card)] text-[13px] ${rtl ? "pl-11" : "pr-11"}`}
                        placeholder={t.passwordPlaceholder}
                        aria-label={t.password}
                      />
                      <button
                        type="button"
                        onClick={() => setShowPassword((v) => !v)}
                        className={`absolute inset-y-0 grid w-11 place-items-center text-[var(--muted)] transition hover:text-[var(--fg)] ${rtl ? "left-0" : "right-0"}`}
                        aria-label={
                          showPassword ? "Hide password" : "Show password"
                        }
                      >
                        {showPassword ? (
                          <EyeOff className="h-4 w-4" />
                        ) : (
                          <Eye className="h-4 w-4" />
                        )}
                      </button>
                    </label>
                  </>
                ) : (
                  <label className="relative block">
                    <span className="sr-only">{t.code}</span>
                    <KeyRound
                      className={`pointer-events-none absolute top-3.5 h-4 w-4 text-[var(--muted)] ${rtl ? "right-3.5" : "left-3.5"}`}
                    />
                    <Input
                      autoFocus
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      value={code}
                      onChange={(e) => setCode(e.target.value)}
                      className={`input h-11 bg-[var(--card)] text-[13px] tracking-[.16em] ${rtl ? "pr-10" : "pl-10"}`}
                      placeholder={t.codePlaceholder}
                      aria-label={t.code}
                    />
                  </label>
                )}

                {error && (
                  <div
                    role="alert"
                    className="rounded-lg border border-red-500/20 bg-red-500/8 px-3 py-2.5 text-[12px] leading-5 text-red-500"
                  >
                    {error}
                  </div>
                )}

                <Button
                  type="submit"
                  className="h-11 w-full border-[var(--brand)] bg-[var(--brand)] text-[13px] text-black hover:bg-[var(--brand-strong)] hover:opacity-100"
                  disabled={!canSubmit}
                >
                  {busy ? (
                    <>
                      <LoaderCircle className="h-4 w-4 animate-spin" />
                      {t.loading}
                    </>
                  ) : (
                    <>
                      <LogIn className="h-4 w-4" />
                      {need2fa ? t.verify : t.signIn}
                    </>
                  )}
                </Button>

                {passkeyEnabled && !need2fa && (
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full"
                    disabled={busy}
                    onClick={() => void startPasskey()}
                  >
                    <Fingerprint className="h-4 w-4 text-emerald-500" />
                    {locale === "fa"
                      ? "ورود با کلید ورود"
                      : "Sign in with passkey"}
                  </Button>
                )}
                {oidcEnabled && !need2fa && (
                  <Button
                    variant="outline"
                    className="w-full"
                    disabled={busy}
                    onClick={() => void startSSO("oidc")}
                  >
                    {telegram
                      ? locale === "fa"
                        ? "ورود با تلگرام"
                        : "Sign in with Telegram"
                      : locale === "fa"
                        ? "ورود با حساب سازمانی"
                        : "Sign in with SSO"}
                  </Button>
                )}
                {githubEnabled && !need2fa && (
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full"
                    disabled={busy}
                    onClick={() => void startSSO("github")}
                  >
                    {locale === "fa"
                      ? "ورود با گیت‌هاب"
                      : "Sign in with GitHub"}
                  </Button>
                )}
                {need2fa && (
                  <button
                    type="button"
                    onClick={backToPassword}
                    className="flex h-9 w-full items-center justify-center gap-2 rounded-md text-[11px] font-medium text-[var(--muted)] transition hover:bg-[var(--surface-2)] hover:text-[var(--fg)]"
                  >
                    <ArrowLeft
                      className={`h-3.5 w-3.5 ${rtl ? "rotate-180" : ""}`}
                    />
                    {t.back}
                  </button>
                )}
              </form>
            </div>
          </div>
        </section>

        <footer className="flex flex-col items-center gap-1.5 pb-1 text-center">
          <div className="text-[9px] font-medium uppercase tracking-[.12em] text-[var(--muted)] opacity-80">
            {t.version}
          </div>
        </footer>
      </div>
    </main>
  );
}
