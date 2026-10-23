import { useEffect, useState } from "react";
import { Server, RefreshCw } from "lucide-react";
import { api } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
type Instance = {
  id: string;
  hostname: string;
  role: string;
  heartbeat_at: string;
};
export function PanelCluster() {
  useLocale();
  const [instances, setInstances] = useState<Instance[]>([]),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  async function load(signal?: AbortSignal) {
    setBusy(true);
    try {
      const d = await api<{ instances: Instance[] }>("/cluster", { signal });
      if (!signal?.aborted) {
        setInstances(d.instances);
        setError("");
      }
    } catch (e) {
      if (!signal?.aborted)
        setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (!signal?.aborted) setBusy(false);
    }
  }
  useEffect(() => {
    const c = new AbortController();
    void load(c.signal);
    return () => c.abort();
  }, []);
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("Panel availability", "دسترس‌پذیری پنل")}</CardTitle>
        <Button
          variant="outline"
          disabled={busy}
          aria-label={t("Refresh panel instances", "تازه‌سازی نمونه‌های پنل")}
          onClick={() => void load()}
        >
          <RefreshCw className={`h-4 w-4 ${busy ? "animate-spin" : ""}`} />
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <p role="alert" className="text-[12px] text-red-500">
            {error}
          </p>
        )}
        {instances.map((i) => (
          <div className="flex items-center gap-3 text-[12px]" key={i.id}>
            <Server className="h-4 w-4 text-[var(--muted)]" />
            <div className="min-w-0 flex-1">
              <p className="truncate">{i.hostname}</p>
              <p className="mt-1 text-[10px] text-[var(--muted)]">
                {new Date(i.heartbeat_at).toLocaleString()}
              </p>
            </div>
            <span
              className={
                i.role === "active" ? "text-emerald-500" : "text-[var(--muted)]"
              }
            >
              {i.role === "active"
                ? t("Active", "فعال")
                : i.role === "standby"
                  ? t("Standby", "آماده‌به‌کار")
                  : t("Offline", "آفلاین")}
            </span>
          </div>
        ))}
        <p className="text-[11px] leading-6 text-[var(--muted)]">
          {t(
            "One panel runs background jobs. A standby takes over after the database session is released. HTTP failover requires a load balancer.",
            "کارهای پس‌زمینه روی یک پنل اجرا می‌شوند. پس از آزاد شدن نشست دیتابیس، نمونهٔ آماده‌به‌کار جایگزین می‌شود. جابه‌جایی ترافیک وب به لودبالانسر نیاز دارد.",
          )}
        </p>
      </CardContent>
    </Card>
  );
}
