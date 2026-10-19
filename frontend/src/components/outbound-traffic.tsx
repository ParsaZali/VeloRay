import { formatDate } from "@/lib/dates";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/controls";

type Traffic = {
  available: boolean;
  collected_at: string | null;
  outbounds: {
    tag: string;
    upload_bytes: number;
    download_bytes: number;
    period_upload_bytes: number;
    period_download_bytes: number;
  }[];
};
const gib = (n: number) => (n / 1073741824).toFixed(3);

export function OutboundTraffic({
  node,
  core,
}: {
  node: number;
  core: string;
}) {
  useLocale();
  const [open, setOpen] = useState(false),
    [hours, setHours] = useState(24),
    [revision, setRevision] = useState(0),
    [data, setData] = useState<Traffic | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    setBusy(true);
    setError("");
    void api<Traffic>(
      `/outbound-traffic?node=${node}&core=${core}&hours=${hours}`,
      { signal: controller.signal },
    )
      .then((result) => {
        if (!controller.signal.aborted) setData(result);
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
    return () => controller.abort();
  }, [open, node, core, hours, revision]);
  return (
    <details
      className="rounded-lg border border-[var(--border)] bg-[var(--card)] p-4"
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary className="cursor-pointer text-[13px] font-medium">
        {t("Outbound traffic")}
      </summary>
      {open && (
        <div className="mt-4 space-y-3">
          <p className="text-[12px] text-[var(--muted)]">
            {t(
              "Independent raw counters per core and tag. These totals do not consume customer quota and may include several hops.",
            )}
          </p>
          <div className="flex flex-wrap items-end gap-3">
            <label className="label">
              {t("Traffic period")}
              <Select value={hours} onChange={(e) => setHours(+e.target.value)}>
                <option value={24}>{t("24 hours")}</option>
                <option value={168}>{t("7 days")}</option>
                <option value={720}>{t("30 days")}</option>
              </Select>
            </label>
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => setRevision((n) => n + 1)}
            >
              {t("Refresh traffic")}
            </Button>
          </div>
          {busy && <p role="status">{t("Loading…")}</p>}
          {error && (
            <p role="alert" className="text-red-500">
              {error}
            </p>
          )}
          {!busy && !error && data && (
            <>
              {!data.available && (
                <p role="status" className="text-[12px]">
                  {t(
                    "Current outbound telemetry is unavailable. Upgrade the node agent and apply its configuration.",
                  )}
                </p>
              )}
              {data.collected_at && (
                <p className="text-[11px] text-[var(--muted)]">
                  {t("Last collected")}:{" "}
                  <time dateTime={data.collected_at}>
                    {formatDate(data.collected_at, {
                      hour: "2-digit",
                      minute: "2-digit",
                    })}
                  </time>
                </p>
              )}
              <div className="overflow-x-auto">
                <table className="data-table min-w-[620px]">
                  <caption className="sr-only">
                    {t("Outbound traffic")} · {core}
                  </caption>
                  <thead>
                    <tr>
                      <th scope="col">{t("Tag")}</th>
                      <th scope="col">{t("Period upload (GiB)")}</th>
                      <th scope="col">{t("Period download (GiB)")}</th>
                      <th scope="col">{t("Tracked upload (GiB)")}</th>
                      <th scope="col">{t("Tracked download (GiB)")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.outbounds.map((row) => (
                      <tr key={row.tag}>
                        <th scope="row" className="font-mono text-start">
                          {row.tag}
                        </th>
                        <td>{gib(row.period_upload_bytes)}</td>
                        <td>{gib(row.period_download_bytes)}</td>
                        <td>{gib(row.upload_bytes)}</td>
                        <td>{gib(row.download_bytes)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {!data.outbounds.length && (
                <p className="text-[12px]">
                  {t("No outbound traffic recorded yet.")}
                </p>
              )}
              <p className="text-[11px] text-[var(--muted)]">
                {t(
                  "Tracked totals start with the first durable agent sample; period history follows traffic retention. Reusing a tag continues its totals.",
                )}
              </p>
            </>
          )}
        </div>
      )}
    </details>
  );
}
