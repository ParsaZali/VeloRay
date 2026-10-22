import { useEffect, useRef, useState } from "react";
import {
  Cpu,
  Download,
  ExternalLink,
  LoaderCircle,
  RotateCcw,
  ShieldCheck,
} from "lucide-react";
import { api } from "@/lib/api";
import { useSession } from "@/lib/session";
import { t, useLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/controls";

type Core = "xray" | "singbox";
type Release = { tag: string; sha256: string; url: string };
type CoreState = {
  tag: string;
  source: string;
  sha256: string;
  can_rollback: boolean;
  user_ip_attribution: boolean;
};
type State = {
  managed: boolean;
  enabled: boolean;
  reason?: string;
  cores?: Partial<Record<Core, CoreState>>;
  job?: { core: Core; tag: string; state: string; error?: string };
};
const repositories = { xray: "XTLS/Xray-core", singbox: "SagerNet/sing-box" };

export function CoreManager({ nodeId }: { nodeId: string }) {
  useLocale();
  const session = useSession();
  const [state, setState] = useState<State | null>(null);
  const [releases, setReleases] = useState<Partial<Record<Core, Release[]>>>(
    {},
  );
  const [selected, setSelected] = useState<Partial<Record<Core, string>>>({});
  const [revision, refresh] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const running = state?.job?.state === "running";
  const allowed = !!session?.is_superuser;
  useEffect(() => {
    if (!nodeId || !allowed) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      try {
        const next = await api<State>(`/nodes/${nodeId}/core-manager/`, {
          signal: controller.signal,
        });
        if (controller.signal.aborted) return;
        setState(next);
        if (next.job?.state === "running")
          timer = setTimeout(() => void load(), 2000);
      } catch (e) {
        if (!controller.signal.aborted)
          setError(
            e instanceof Error
              ? e.message
              : t("Core manager unavailable", "مدیریت هسته در دسترس نیست"),
          );
      }
    }
    void load();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [nodeId, allowed, revision]);
  const controller = useRef(new AbortController());
  useEffect(() => {
    controller.current = new AbortController();
    return () => controller.current.abort();
  }, []);
  async function catalog(core: Core) {
    setBusy(true);
    setError("");
    try {
      const result = await api<{ releases: Release[] }>(
        `/nodes/${nodeId}/core-releases/?core=${core}`,
        { signal: controller.current.signal },
      );
      if (controller.current.signal.aborted) return;
      setReleases((old) => ({ ...old, [core]: result.releases }));
      setSelected((old) => ({ ...old, [core]: result.releases[0]?.tag || "" }));
      if (!result.releases.length)
        setError(
          t(
            "No verified stable release is available for this architecture.",
            "نسخه پایدار تأییدشده‌ای برای این معماری پیدا نشد.",
          ),
        );
    } catch (e) {
      if (!controller.current.signal.aborted)
        setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (!controller.current.signal.aborted) setBusy(false);
    }
  }
  async function policy(enabled: boolean) {
    setBusy(true);
    setError("");
    try {
      const next = await api<State>(`/nodes/${nodeId}/core-policy/`, {
        method: "POST",
        body: JSON.stringify({ enabled }),
        signal: controller.current.signal,
      });
      if (!controller.current.signal.aborted) setState(next);
    } catch (e) {
      if (!controller.current.signal.aborted)
        setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (!controller.current.signal.aborted) setBusy(false);
    }
  }
  async function install(core: Core, rollback = false) {
    if (
      !confirm(
        t(
          "Change the core version? Active connections may reconnect. The previous executable is retained for rollback.",
          "نسخه هسته تغییر کند؟ اتصال‌های فعال ممکن است دوباره وصل شوند. فایل نسخه قبلی برای بازگشت حفظ می‌شود.",
        ),
      )
    )
      return;
    setBusy(true);
    setError("");
    try {
      const next = await api<State>(`/nodes/${nodeId}/core-install/`, {
        method: "POST",
        body: JSON.stringify({ core, tag: selected[core] || "", rollback }),
        signal: controller.current.signal,
      });
      if (!controller.current.signal.aborted) {
        setState(next);
        refresh((value) => value + 1);
      }
    } catch (e) {
      if (!controller.current.signal.aborted)
        setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (!controller.current.signal.aborted) setBusy(false);
    }
  }
  if (!allowed) return null;
  return (
    <section
      className="core-manager"
      aria-label={t("Core versions", "نسخه هسته‌ها")}
    >
      <header>
        <div>
          <ShieldCheck size={17} />
          <h3>{t("Core versions", "نسخه هسته‌ها")}</h3>
        </div>
        {state?.managed && (
          <label className="core-policy">
            <Input
              type="checkbox"
              aria-label={t(
                "Allow official core downloads",
                "اجازه دانلود هسته رسمی",
              )}
              checked={state.enabled}
              disabled={busy || running}
              onChange={(event) => void policy(event.target.checked)}
            />
            {t("Allow official downloads", "اجازه دانلود رسمی")}
          </label>
        )}
      </header>
      {!state && !error && <p className="core-message">{t("Checking…")}</p>}
      {state && !state.managed && (
        <p className="core-message">
          {t(
            "Update the node installation to manage core versions. Container deployments update through their image.",
            "برای مدیریت نسخه هسته، نصب نود را ارتقا دهید. نسخه کانتینری از طریق ایمیج به‌روزرسانی می‌شود.",
          )}
        </p>
      )}
      {state?.managed && (
        <>
          <div className="core-versions">
            {(["xray", "singbox"] as Core[])
              .filter((core) => state.cores?.[core])
              .map((core) => {
                const current = state.cores![core]!;
                return (
                  <article key={core}>
                    <div className="core-title">
                      <Cpu size={16} />
                      <strong>{core === "xray" ? "Xray" : "sing-box"}</strong>
                      <span>
                        {current.source === "official"
                          ? current.tag
                          : t("Bundled", "همراه پنل")}
                      </span>
                    </div>
                    <a
                      href={`https://github.com/${repositories[core]}/releases`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="core-source"
                    >
                      {repositories[core]}
                      <ExternalLink size={12} />
                    </a>
                    <div className="core-actions">
                      {!releases[core] ? (
                        <Button
                          type="button"
                          variant="outline"
                          disabled={busy || running}
                          onClick={() => void catalog(core)}
                        >
                          <Download size={14} />
                          {t("Check releases", "بررسی نسخه‌ها")}
                        </Button>
                      ) : (
                        <>
                          <Select
                            aria-label={`${core} ${t("version", "نسخه")}`}
                            value={selected[core] || ""}
                            disabled={busy || running}
                            onChange={(event) =>
                              setSelected((old) => ({
                                ...old,
                                [core]: event.target.value,
                              }))
                            }
                          >
                            {releases[core]!.map((release) => (
                              <option key={release.tag} value={release.tag}>
                                {release.tag}
                              </option>
                            ))}
                          </Select>
                          <Button
                            type="button"
                            disabled={
                              busy ||
                              running ||
                              !state.enabled ||
                              !selected[core]
                            }
                            onClick={() => void install(core)}
                          >
                            <Download size={14} />
                            {t("Install version", "نصب نسخه")}
                          </Button>
                        </>
                      )}
                      {current.can_rollback && (
                        <Button
                          type="button"
                          variant="outline"
                          disabled={busy || running}
                          onClick={() => void install(core, true)}
                        >
                          <RotateCcw size={14} />
                          {t("Rollback", "بازگشت")}
                        </Button>
                      )}
                    </div>
                    {current.sha256 && (
                      <details className="core-checksum">
                        <summary>Binary SHA-256</summary>
                        <code dir="ltr">{current.sha256}</code>
                      </details>
                    )}
                  </article>
                );
              })}
          </div>
          <footer>
            {t(
              "Manual updates · Official GitHub releases · SHA-256 verified",
              "آپدیت دستی · نسخه رسمی گیت‌هاب · تأیید SHA-256",
            )}
          </footer>
          {selected.singbox && (
            <p className="core-message">
              {t(
                "Per-user IP limits require the bundled sing-box build. Incompatible versions are rejected before replacement.",
                "محدودیت IP هر کاربر به sing-box همراه پنل نیاز دارد. نسخه ناسازگار پیش از جایگزینی رد می‌شود.",
              )}
            </p>
          )}
          {running && (
            <p className="core-message" role="status">
              <LoaderCircle className="animate-spin" size={14} />
              {t("Installing verified core…", "در حال نصب هسته تأییدشده…")}{" "}
              {state.job?.tag}
            </p>
          )}
          {state.job?.state === "success" && (
            <p className="core-success" role="status">
              {t(
                "Core version changed successfully.",
                "نسخه هسته با موفقیت تغییر کرد.",
              )}
            </p>
          )}
          {state.job?.state === "error" && (
            <p className="core-error" role="alert">
              {state.job.error}
            </p>
          )}
        </>
      )}
      {error && (
        <p className="core-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
