import { useEffect, useState } from "react";
import { ExternalLink, Network } from "lucide-react";
import { api } from "@/lib/api";
import type { HostJob } from "@/lib/panel-updates";
import { notify } from "@/lib/notifications";
import { t } from "@/lib/i18n";
import { Input } from "@/components/ui/controls";
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Dialog } from "@/components/common/dialog";
type Endpoint = {
  port: number;
  origin: string;
  managed: boolean;
  job: HostJob | null;
};
export function PanelEndpoint() {
  const [endpoint, setEndpoint] = useState<Endpoint | null>(null),
    [port, setPort] = useState(""),
    [error, setError] = useState(""),
    [confirm, setConfirm] = useState(false),
    [busy, setBusy] = useState(false),
    [next, setNext] = useState("");
  useEffect(() => {
    const c = new AbortController();
    void api<Endpoint>("/panel-endpoint", { signal: c.signal })
      .then((data) => {
        if (!c.signal.aborted) {
          setEndpoint(data);
          setPort(String(data.port));
        }
      })
      .catch((e) => {
        if (!c.signal.aborted)
          setError(e instanceof Error ? e.message : String(e));
      });
    return () => c.abort();
  }, []);
  async function apply() {
    if (!endpoint) return;
    setBusy(true);
    setError("");
    try {
      const address = new URL(endpoint.origin);
      address.port = port;
      await api("/panel-endpoint", {
        method: "PUT",
        body: JSON.stringify({ port: Number(port) }),
      });
      setNext(address.origin);
      setConfirm(false);
      notify(
        t(
          "Panel port change queued. Use the new address after the service restarts.",
          "تغییر پورت پنل در صف است. پس از راه‌اندازی سرویس از آدرس جدید استفاده کنید.",
        ),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  const valid =
    Number.isInteger(Number(port)) &&
    Number(port) > 0 &&
    Number(port) < 65536 &&
    Number(port) !== endpoint?.port;
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <span className="inline-flex items-center gap-2">
            <Network size={16} />
            {t("Panel port", "پورت پنل")}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-end gap-2">
          <label className="label w-full sm:max-w-48">
            {t("HTTPS port", "پورت HTTPS")}
            <Input
              type="number"
              min={1}
              max={65535}
              value={port}
              disabled={!endpoint?.managed || busy || Boolean(next)}
              onChange={(e) => setPort(e.target.value)}
              dir="ltr"
            />
          </label>
          <Button
            type="button"
            disabled={!endpoint?.managed || busy || !valid || Boolean(next)}
            onClick={() => setConfirm(true)}
          >
            {t("Apply port")}
          </Button>
        </div>
        {endpoint && !next && (
          <code
            dir="ltr"
            className="block break-all text-[12px] text-[var(--muted)]"
          >
            {endpoint.origin}
          </code>
        )}
        {next && (
          <div role="status" className="space-y-2 text-[12px]">
            <p>
              {t(
                "The address will change after the service restarts.",
                "پس از راه‌اندازی سرویس، آدرس تغییر می‌کند.",
              )}
            </p>
            <a
              className="inline-flex items-center gap-2 break-all text-[var(--brand)]"
              href={next}
              rel="noreferrer"
            >
              {next}
              <ExternalLink size={14} />
            </a>
          </div>
        )}
        <p className="text-[12px] text-[var(--muted)]">
          {t(
            "Allow the port in your firewall. VPN listening ports are edited under Inbounds.",
            "پورت را در فایروال باز کنید. پورت‌های VPN از بخش Inbounds قابل تغییرند.",
          )}
        </p>
        {error && (
          <p role="alert" className="text-[12px] text-red-500">
            {error}
          </p>
        )}
        {confirm && (
          <Dialog
            label={t("Change panel port", "تغییر پورت پنل")}
            size="sm"
            onClose={() => !busy && setConfirm(false)}
          >
            <div className="space-y-4">
              <h2>{t("Change panel port", "تغییر پورت پنل")}</h2>
              <p className="text-[13px] leading-6">
                {t(
                  "The panel restarts on the new HTTPS port. Sign in at its new address.",
                  "پنل روی پورت HTTPS جدید راه‌اندازی می‌شود. با آدرس جدید دوباره وارد شوید.",
                )}
              </p>
              <code dir="ltr" className="block text-sm">
                {port}
              </code>
              {error && (
                <p role="alert" className="text-sm text-red-500">
                  {error}
                </p>
              )}
              <div className="flex justify-end gap-2">
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() => setConfirm(false)}
                >
                  {t("Cancel")}
                </Button>
                <Button
                  type="button"
                  disabled={busy}
                  onClick={() => void apply()}
                >
                  {t("Apply port")}
                </Button>
              </div>
            </div>
          </Dialog>
        )}
      </CardContent>
    </Card>
  );
}
