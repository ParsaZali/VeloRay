import { useEffect, useState } from "react";
import { Box, Copy, Check, Server, ExternalLink } from "lucide-react";
import { api } from "@/lib/api";
import { t } from "@/lib/i18n";
import { notify } from "@/lib/notifications";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

type Deployment = {
  mode: "docker" | "native" | "external";
  database: string;
  https_proxy: string;
  architecture: string;
  panel_origin: string;
  managed_updates: boolean;
};
const commands = [
  ["Health check", "بررسی سلامت", "sudo veloray doctor"],
  ["Create backup", "ساخت پشتیبان", "sudo veloray backup"],
  ["View logs", "مشاهدهٔ گزارش", "sudo veloray logs web"],
  ["Update VeloRay", "به‌روزرسانی VeloRay", "sudo veloray update"],
] as const;

export function DeploymentStatus() {
  const [data, setData] = useState<Deployment | null>(null);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    void api<Deployment>("/deployment", { signal: controller.signal })
      .then((result) => {
        if (!controller.signal.aborted) setData(result);
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : String(e));
      });
    return () => controller.abort();
  }, []);
  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(() => setCopied(""), 1800);
    return () => window.clearTimeout(timer);
  }, [copied]);
  async function copy(command: string) {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(command);
      notify(t("Copied", "کپی شد"), "success");
    } catch {
      notify(
        t(
          "Could not copy. Select the command manually.",
          "کپی نشد؛ دستور را انتخاب و کپی کنید.",
        ),
        "error",
      );
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <span className="inline-flex items-center gap-2">
            {data?.mode === "docker" ? <Box size={17} /> : <Server size={17} />}
            {t("Deployment", "نحوهٔ اجرا")}
          </span>
        </CardTitle>
        {data && (
          <span className="rounded-md border border-[var(--border)] px-2 py-1 text-xs text-[var(--muted-strong)]">
            {data.mode === "docker"
              ? "Docker Compose"
              : data.mode === "native"
                ? "systemd"
                : t("External", "خارجی")}
          </span>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        {error ? (
          <p role="alert" className="text-sm text-red-500">
            {error}
          </p>
        ) : !data ? (
          <p role="status" className="text-sm text-[var(--muted)]">
            {t("Loading…", "در حال دریافت…")}
          </p>
        ) : (
          <dl className="grid grid-cols-2 gap-3 text-xs sm:grid-cols-3">
            {[
              [t("Database", "پایگاه داده"), data.database],
              [t("HTTPS proxy", "پروکسی HTTPS"), data.https_proxy],
              [t("Architecture", "معماری"), data.architecture],
            ].map(([label, value]) => (
              <div key={label} className="min-w-0">
                <dt className="mb-1 text-[var(--muted)]">{label}</dt>
                <dd dir="ltr" className="font-medium">
                  {value}
                </dd>
              </div>
            ))}
          </dl>
        )}
        <div className="grid gap-2 sm:grid-cols-2">
          {commands.map(([label, fa, command]) => (
            <div
              key={command}
              className="flex min-w-0 items-center gap-2 rounded-lg border border-[var(--border)] px-3 py-2.5"
            >
              <div className="min-w-0 flex-1">
                <p className="mb-1 text-xs text-[var(--muted)]">
                  {t(label, fa)}
                </p>
                <code dir="ltr" className="block select-all break-all text-xs">
                  {command}
                </code>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label={`${t("Copy", "کپی")} ${command}`}
                onClick={() => void copy(command)}
              >
                {copied === command ? (
                  <Check size={15} className="text-[var(--brand)]" />
                ) : (
                  <Copy size={15} />
                )}
              </Button>
            </div>
          ))}
        </div>
        {data?.mode === "docker" && (
          <a
            className="inline-flex items-center gap-1.5 text-xs text-[var(--brand)]"
            href="https://github.com/ParsaZali/VeloRay/blob/main/docs/container-deployment.md"
            target="_blank"
            rel="noreferrer"
          >
            {t("Docker deployment guide", "راهنمای اجرای Docker")}
            <ExternalLink size={13} />
          </a>
        )}
      </CardContent>
    </Card>
  );
}
