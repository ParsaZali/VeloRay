import { useEffect, useRef, useState } from "react";
import {
  ChevronDown,
  Database,
  Download,
  ExternalLink,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
} from "lucide-react";
import { api } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { useSession } from "@/lib/session";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/controls";
import { Dialog } from "@/components/common/dialog";

type Kind = "geoip" | "geosite";
type Release = { tag: string; sha256: string; url: string };
type Asset = {
  tag: string;
  source: string;
  sha256: string;
  available: boolean;
  repository: string;
  can_rollback: boolean;
};
type State = {
  managed: boolean;
  enabled: boolean;
  core_busy?: boolean;
  assets?: Record<Kind, Asset>;
  job?: { kind: Kind; tag: string; state: string; error?: string };
};
const sources = {
  geoip: "v2fly/geoip",
  geosite: "v2fly/domain-list-community",
};
const names = { geoip: "GeoIP", geosite: "Geosite" };
export function GeoManager({
  nodeId,
  externalRevision = 0,
  onChange,
}: {
  nodeId: string;
  externalRevision?: number;
  onChange?: () => void;
}) {
  useLocale();
  const session = useSession();
  const allowed = !!session?.is_superuser;
  const [expanded, setExpanded] = useState(false);
  const [state, setState] = useState<State | null>(null);
  const [releases, setReleases] = useState<Partial<Record<Kind, Release[]>>>(
    {},
  );
  const [selected, setSelected] = useState<Partial<Record<Kind, string>>>({});
  const [revision, refresh] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState<{
    kind: Kind;
    tag: string;
    rollback: boolean;
  } | null>(null);
  const action = useRef<AbortController | null>(null);
  const running = state?.job?.state === "running" || !!state?.core_busy;
  useEffect(() => () => action.current?.abort(), []);
  useEffect(() => {
    if (!expanded || !allowed || !nodeId) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      try {
        const next = await api<State>(`/nodes/${nodeId}/geo-manager/`, {
          signal: controller.signal,
        });
        if (controller.signal.aborted) return;
        if (
          typeof next.managed !== "boolean" ||
          (next.managed && (!next.assets?.geoip || !next.assets?.geosite))
        )
          throw new Error(
            t(
              "Update the node agent to manage geo assets.",
              "برای مدیریت فایل‌های Geo، عامل نود را به‌روز کنید.",
            ),
          );
        setState(next);
        setError("");
        if (next.job?.state === "running" || next.core_busy)
          timer = setTimeout(() => void load(), 2000);
      } catch (e) {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : String(e));
      }
    }
    void load();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [expanded, allowed, nodeId, revision, externalRevision]);
  async function catalog(kind: Kind) {
    action.current?.abort();
    const controller = new AbortController();
    action.current = controller;
    setBusy(true);
    setError("");
    try {
      const [result, next] = await Promise.all([
        api<{ releases: Release[] }>(
          `/nodes/${nodeId}/geo-releases/?kind=${kind}`,
          { signal: controller.signal },
        ),
        api<State>(`/nodes/${nodeId}/geo-manager/`, {
          signal: controller.signal,
        }),
      ]);
      if (controller.signal.aborted) return;
      if (!Array.isArray(result.releases))
        throw new Error(
          t(
            "Geo releases are unavailable.",
            "فهرست نسخه‌های Geo در دسترس نیست.",
          ),
        );
      setState(next);
      setReleases((old) => ({ ...old, [kind]: result.releases }));
      setSelected((old) => ({ ...old, [kind]: result.releases[0]?.tag || "" }));
      if (!result.releases.length)
        setError(
          t(
            "No verified stable release is available.",
            "نسخهٔ پایدار تأییدشده‌ای پیدا نشد.",
          ),
        );
    } catch (e) {
      if (!controller.signal.aborted)
        setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  async function install() {
    if (!pending) return;
    const request = pending;
    setPending(null);
    action.current?.abort();
    const controller = new AbortController();
    action.current = controller;
    setBusy(true);
    setError("");
    try {
      const next = await api<State>(`/nodes/${nodeId}/geo-install/`, {
        method: "POST",
        body: JSON.stringify(request),
        signal: controller.signal,
      });
      if (!controller.signal.aborted) {
        setState(next);
        refresh((value) => value + 1);
        onChange?.();
      }
    } catch (e) {
      if (!controller.signal.aborted)
        setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  if (!allowed) return null;
  return (
    <section
      className="overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--card)]"
      aria-label={t("Geo asset updates", "به‌روزرسانی فایل‌های Geo")}
    >
      <button
        type="button"
        className="flex w-full items-center gap-2 px-4 py-4 text-start text-[14px] font-semibold"
        aria-expanded={expanded}
        aria-controls={`geo-manager-${nodeId}`}
        onClick={() => setExpanded((value) => !value)}
      >
        <Database className="h-4 w-4 text-[var(--muted)]" />
        GeoIP / Geosite
        <ChevronDown
          className={`ms-auto h-4 w-4 transition-transform ${expanded ? "rotate-180" : ""}`}
        />
      </button>
      {expanded && (
        <div
          id={`geo-manager-${nodeId}`}
          className="space-y-4 border-t border-[var(--border)] p-4"
        >
          <div className="flex items-center justify-between gap-3">
            <p className="text-[12px] leading-6 text-[var(--muted)]">
              {t(
                "Verified official releases with a previous-version rollback.",
                "نسخه‌های رسمی با بررسی صحت فایل و امکان بازگشت.",
              )}
            </p>
            <Button
              type="button"
              variant="ghost"
              disabled={busy}
              aria-label={t("Refresh geo status", "تازه‌سازی وضعیت Geo")}
              onClick={() => refresh((value) => value + 1)}
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
          </div>
          {error && (
            <p role="alert" className="text-[12px] text-red-500">
              {error}
            </p>
          )}
          {!state && !error && (
            <p
              role="status"
              className="flex items-center gap-2 text-[12px] text-[var(--muted)]"
            >
              <LoaderCircle className="h-4 w-4 animate-spin" />
              {t("Checking…")}
            </p>
          )}
          {state && !state.managed && (
            <p className="text-[12px] text-[var(--muted)]">
              {t(
                "Update the managed node installation. Containers update geo assets through their image.",
                "نصب نود را به‌روز کنید. در نصب کانتینری فایل‌ها از طریق ایمیج به‌روز می‌شوند.",
              )}
            </p>
          )}
          {state?.managed && (
            <>
              {!state.enabled && (
                <p className="rounded-md bg-[var(--surface-2)] px-3 py-2 text-[12px]">
                  {t(
                    "Enable official downloads in Core versions to install a new release.",
                    "برای نصب نسخهٔ جدید، دانلود رسمی را در بخش نسخهٔ هسته‌ها فعال کنید.",
                  )}
                </p>
              )}
              <div className="grid gap-3 sm:grid-cols-2">
                {(["geoip", "geosite"] as Kind[]).map((kind) => {
                  const current = state.assets?.[kind];
                  const checked = releases[kind]?.find(
                    (item) => item.tag === selected[kind],
                  );
                  return (
                    <article
                      key={kind}
                      className="min-w-0 space-y-3 rounded-lg border border-[var(--border)] p-4"
                      aria-label={names[kind]}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <h4 className="text-[13px] font-semibold">
                          {names[kind]}
                        </h4>
                        <span className="max-w-[65%] break-all rounded border border-[var(--border)] px-2 py-1 text-[11px] text-[var(--muted)]">
                          {!current?.available
                            ? t("Unavailable", "در دسترس نیست")
                            : current.source === "installed"
                              ? t("Installed", "نصب‌شده")
                              : current.tag}
                        </span>
                      </div>
                      <a
                        href={`https://github.com/${sources[kind]}/releases`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="flex items-center gap-1 break-all text-[11px] text-[var(--muted)]"
                        dir="ltr"
                      >
                        {sources[kind]}
                        <ExternalLink className="h-3 w-3 shrink-0" />
                      </a>
                      <div className="flex flex-wrap items-center gap-2">
                        {!releases[kind] ? (
                          <Button
                            type="button"
                            variant="outline"
                            disabled={busy || running}
                            onClick={() => void catalog(kind)}
                          >
                            <Download className="h-4 w-4" />
                            {t("Check releases", "بررسی نسخه‌ها")}
                          </Button>
                        ) : (
                          <>
                            <Select
                              className="input min-w-0 flex-1"
                              aria-label={`${names[kind]} ${t("version", "نسخه")}`}
                              value={selected[kind] || ""}
                              disabled={busy || running}
                              onChange={(e) =>
                                setSelected((old) => ({
                                  ...old,
                                  [kind]: e.target.value,
                                }))
                              }
                            >
                              {releases[kind]!.map((item) => (
                                <option key={item.tag} value={item.tag}>
                                  {item.tag}
                                </option>
                              ))}
                            </Select>
                            <Button
                              type="button"
                              disabled={
                                busy || running || !state.enabled || !checked
                              }
                              onClick={() =>
                                setPending({
                                  kind,
                                  tag: selected[kind] || "",
                                  rollback: false,
                                })
                              }
                            >
                              <Download className="h-4 w-4" />
                              {t("Update", "به‌روزرسانی")}
                            </Button>
                          </>
                        )}
                        {current?.can_rollback && (
                          <Button
                            type="button"
                            variant="outline"
                            disabled={busy || running}
                            onClick={() =>
                              setPending({ kind, tag: "", rollback: true })
                            }
                          >
                            <RotateCcw className="h-4 w-4" />
                            {t("Rollback", "بازگشت")}
                          </Button>
                        )}
                      </div>
                      {(current?.sha256 || checked) && (
                        <details className="text-[10px] text-[var(--muted)]">
                          <summary>SHA-256</summary>
                          {current?.sha256 && (
                            <p className="mt-2">
                              {t("Installed", "نصب‌شده")}
                              <code dir="ltr" className="mt-1 block break-all">
                                {current.sha256}
                              </code>
                            </p>
                          )}
                          {checked && (
                            <p className="mt-2">
                              {t("Selected", "انتخاب‌شده")}
                              <code dir="ltr" className="mt-1 block break-all">
                                {checked.sha256}
                              </code>
                            </p>
                          )}
                        </details>
                      )}
                    </article>
                  );
                })}
              </div>
              {running && (
                <p
                  role="status"
                  className="flex items-center gap-2 text-[12px] text-[var(--muted)]"
                >
                  <LoaderCircle className="h-4 w-4 animate-spin" />
                  {t(
                    "A core or geo operation is running…",
                    "عملیات هسته یا فایل Geo در حال انجام است…",
                  )}
                </p>
              )}
              {!running && state.job?.state === "success" && (
                <p role="status" className="text-[12px] text-emerald-500">
                  {t(
                    "Geo asset updated successfully.",
                    "فایل Geo با موفقیت به‌روز شد.",
                  )}
                </p>
              )}
              {!running && state.job?.state === "error" && (
                <p
                  role="alert"
                  className="break-words text-[12px] text-red-500"
                >
                  {state.job.error}
                </p>
              )}
            </>
          )}
        </div>
      )}
      {pending && (
        <Dialog
          label={t("Confirm geo update", "تأیید به‌روزرسانی Geo")}
          onClose={() => setPending(null)}
          size="sm"
        >
          <h3 className="text-[15px] font-semibold">
            {pending.rollback
              ? t("Restore previous asset", "بازگشت به فایل قبلی")
              : t("Update geo asset", "به‌روزرسانی فایل Geo")}
          </h3>
          <p className="mt-3 text-[13px]">
            {names[pending.kind]}
            {pending.tag ? ` · ${pending.tag}` : ""}
          </p>
          <p className="mt-2 text-[12px] leading-6 text-[var(--muted)]">
            {t(
              "Xray connections may reconnect. The current file is retained for rollback.",
              "ممکن است اتصال‌های Xray دوباره وصل شوند. فایل فعلی برای بازگشت حفظ می‌شود.",
            )}
          </p>
          <div className="mt-5 flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => setPending(null)}
            >
              {t("Cancel", "انصراف")}
            </Button>
            <Button type="button" onClick={() => void install()}>
              {t("Confirm", "تأیید")}
            </Button>
          </div>
        </Dialog>
      )}
    </section>
  );
}
