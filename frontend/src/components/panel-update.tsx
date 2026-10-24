import { useEffect, useRef, useState } from "react";
import {
  Download,
  RefreshCw,
  LoaderCircle,
  CheckCircle2,
  ExternalLink,
} from "lucide-react";
import { api } from "@/lib/api";
import { usePanelUpdates } from "@/lib/panel-updates";
import { notify } from "@/lib/notifications";
import { t } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Dialog } from "@/components/common/dialog";
export function PanelUpdate() {
  const { status, error, busy, refresh, running } = usePanelUpdates(true);
  const [confirm, setConfirm] = useState(false),
    [installing, setInstalling] = useState(false);
  const seen = useRef<number | null>(null);
  useEffect(() => {
    const job = status?.job;
    if (!job) return;
    if (job.state === "queued" || job.state === "running")
      seen.current = job.id;
    else if (seen.current === job.id) {
      seen.current = null;
      notify(
        job.state === "succeeded"
          ? t(
              "VeloRay updated successfully. Reload the panel to use the new version.",
              "VeloRay به‌روز شد. برای دریافت نسخهٔ جدید صفحه را تازه‌سازی کنید.",
            )
          : job.error,
        job.state === "succeeded" ? "success" : "error",
      );
    }
  }, [status?.job]);
  async function install() {
    if (!status || installing) return;
    setInstalling(true);
    try {
      await api("/updates/install", {
        method: "POST",
        body: JSON.stringify({ commit: status.latest_commit }),
      });
      setConfirm(false);
      notify(
        t(
          "Update queued. The panel may reconnect during installation.",
          "آپدیت در صف قرار گرفت. هنگام نصب ممکن است ارتباط پنل موقتاً قطع شود.",
        ),
      );
      await refresh();
      window.dispatchEvent(new Event("veloray:update-status"));
    } catch (e) {
      notify(
        e instanceof Error ? e.message : t("Update could not be started"),
        "error",
      );
    } finally {
      setInstalling(false);
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("Panel updates", "به‌روزرسانی پنل")}</CardTitle>
        <span className="text-[12px] text-[var(--muted)]">
          {status?.version ? `v${status.version}` : "—"}
        </span>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-2 text-[12px]">
          {status?.state === "up_to_date" ? (
            <span className="inline-flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400">
              <CheckCircle2 size={15} />
              {t("Up to date", "به‌روز")}
            </span>
          ) : status?.state === "available" ? (
            <span className="text-amber-600 dark:text-amber-400">
              {t("Update available", "آپدیت موجود است")}
            </span>
          ) : (
            <span className="text-[var(--muted)]">
              {busy
                ? t("Checking…", "در حال بررسی…")
                : status?.state === "verifying"
                  ? t("Waiting for CI verification", "در انتظار تأیید CI")
                  : t("Update status unavailable", "وضعیت آپدیت در دسترس نیست")}
            </span>
          )}
        </div>
        <div className="grid gap-3 text-[12px] sm:grid-cols-2">
          <div>
            <span className="block text-[var(--muted)]">
              {t("Installed revision", "نسخهٔ نصب‌شده")}
            </span>
            <code dir="ltr">
              {status?.installed_commit?.slice(0, 12) || "—"}
            </code>
          </div>
          <div>
            <span className="block text-[var(--muted)]">
              {t("Latest revision", "آخرین نسخه")}
            </span>
            <code dir="ltr">{status?.latest_commit?.slice(0, 12) || "—"}</code>
          </div>
        </div>
        {(error || status?.error) && (
          <p role="alert" className="text-[12px] text-red-500">
            {error || status?.error}
          </p>
        )}
        {running && (
          <p role="status" className="flex items-center gap-2 text-[12px]">
            <LoaderCircle size={15} className="animate-spin" />
            {t(
              status?.job?.state === "queued"
                ? "Update queued"
                : "Installing update…",
            )}
          </p>
        )}
        {status?.job?.state === "failed" && (
          <p role="alert" className="text-[12px] text-red-500">
            {status.job.error}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={busy || installing}
            onClick={() => void refresh(true)}
          >
            <RefreshCw size={15} className={busy ? "animate-spin" : ""} />
            {t("Check for updates", "بررسی آپدیت")}
          </Button>
          {status?.managed && (
            <Button
              type="button"
              disabled={status.state !== "available" || running || installing}
              onClick={() => setConfirm(true)}
            >
              <Download size={15} />
              {t("Update VeloRay", "به‌روزرسانی VeloRay")}
            </Button>
          )}
          <a
            href="https://github.com/ParsaZali/VeloRay"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 px-2 text-[12px] text-[var(--muted)]"
          >
            GitHub
            <ExternalLink size={13} />
          </a>
        </div>
        {status && !status.managed && (
          <p className="text-[12px] text-[var(--muted)]">
            {t(
              "Use sudo veloray update to install host control, or update your container image.",
              "برای نصب کنترل میزبان sudo veloray update را اجرا کنید؛ در Docker ایمیج را به‌روز کنید.",
            )}
          </p>
        )}
        {confirm && (
          <Dialog
            label={t("Update VeloRay", "به‌روزرسانی VeloRay")}
            size="sm"
            onClose={() => !installing && setConfirm(false)}
          >
            <div className="space-y-4">
              <h2>{t("Update VeloRay", "به‌روزرسانی VeloRay")}</h2>
              <p className="text-[13px] leading-6 text-[var(--muted-strong)]">
                {t(
                  "Install the verified GitHub revision. A private backup is created first. Services restart during installation; a failed installation attempts to restore the previous application.",
                  "نسخهٔ تأییدشدهٔ گیت‌هاب نصب می‌شود. ابتدا بکاپ خصوصی ساخته می‌شود و سرویس‌ها مجدداً راه‌اندازی می‌شوند. در صورت شکست نصب، بازیابی برنامهٔ قبلی انجام می‌شود.",
                )}
              </p>
              <code dir="ltr" className="block break-all text-[12px]">
                {status?.latest_commit}
              </code>
              <div className="flex justify-end gap-2">
                <Button
                  type="button"
                  variant="outline"
                  disabled={installing}
                  onClick={() => setConfirm(false)}
                >
                  {t("Cancel")}
                </Button>
                <Button
                  type="button"
                  disabled={installing}
                  onClick={() => void install()}
                >
                  {installing && (
                    <LoaderCircle size={14} className="animate-spin" />
                  )}
                  {t("Install update", "نصب آپدیت")}
                </Button>
              </div>
            </div>
          </Dialog>
        )}
      </CardContent>
    </Card>
  );
}
