import { useEffect, useState } from "react";
import { api, type ClientItem } from "@/lib/api";
import { t } from "@/lib/i18n";
import { Dialog } from "@/components/common/dialog";
import { Button } from "@/components/ui/button";
import { can, useSession } from "@/lib/session";
import { Input } from "@/components/ui/controls";
import { X } from "lucide-react";

type Check = {
  hwid_required: boolean;
  hwid_limit: number;
  template_available: boolean;
  checks: {
    inbound: number;
    host_index?: number;
    probe_available?: boolean;
    name: string;
    protocol: string;
    eligible: boolean;
    reasons: string[];
    formats: Record<string, { available: boolean; reason?: string }>;
  }[];
};
export function SubscriptionCheck({
  client,
  onClose,
}: {
  client: ClientItem;
  onClose: () => void;
}) {
  const session = useSession();
  const [probeBusy, setProbeBusy] = useState("");
  const [target, setTarget] = useState("https://www.gstatic.com/generate_204");
  const [probes, setProbes] = useState<
    Record<
      string,
      { ok: boolean; latency_ms?: number; status_code?: number; error?: string }
    >
  >({});
  async function probe(inbound: number, hostIndex: number) {
    const key = `${inbound}:${hostIndex}`;
    setProbeBusy(key);
    try {
      const value = await api<{
        ok: boolean;
        latency_ms?: number;
        status_code?: number;
        error?: string;
      }>(`/clients/${client.id}/connection-test/`, {
        method: "POST",
        body: JSON.stringify({ inbound, host_index: hostIndex, url: target }),
      });
      setProbes((old) => ({ ...old, [key]: value }));
    } catch (error) {
      setProbes((old) => ({
        ...old,
        [key]: {
          ok: false,
          error:
            error instanceof Error
              ? error.message
              : t("Connection test failed"),
        },
      }));
    } finally {
      setProbeBusy("");
    }
  }
  const [result, setResult] = useState<Check | null>(null);
  const [error, setError] = useState("");
  const [history, setHistory] = useState<
    { at: string; format: string; client_app: string; status: number }[]
  >([]);
  useEffect(() => {
    const controller = new AbortController();
    setResult(null);
    setError("");
    setHistory([]);
    setProbes({});
    void api<Check>(`/clients/${client.id}/delivery-check/`, {
      signal: controller.signal,
    })
      .then(setResult)
      .catch((e) => {
        if (!controller.signal.aborted)
          setError(
            e instanceof Error ? e.message : t("Could not check subscription"),
          );
      });
    void api<
      { at: string; format: string; client_app: string; status: number }[]
    >(`/clients/${client.id}/subscription-history/`, {
      signal: controller.signal,
    })
      .then(setHistory)
      .catch(() => {});
    return () => controller.abort();
  }, [client.id]);
  return (
    <Dialog label={t("Check subscription")} onClose={onClose}>
      <header className="mb-5 flex items-center justify-between gap-4">
        <div>
          <h2 className="text-base font-semibold">{t("Check subscription")}</h2>
          <p className="mt-1 text-xs text-[var(--muted)]">{client.name}</p>
        </div>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t("Close")}
          onClick={onClose}
        >
          <X size={16} />
        </Button>
      </header>
      <p className="mb-5 text-xs leading-6 text-[var(--muted)]">
        {t(
          "Checks account policy and export compatibility. This does not test connectivity from a user's network.",
        )}
      </p>
      {error && <p role="alert">{error}</p>}
      {!result && !error && <p role="status">{t("Loading…")}</p>}
      {result && (
        <div className="space-y-4">
          <p className="text-xs">
            {t("HWID required")}: {result.hwid_required ? t("Yes") : t("No")} ·{" "}
            {t("Limit")}: {result.hwid_limit || t("Unlimited")}
          </p>
          {!result.template_available && (
            <p role="status" className="text-xs text-amber-600">
              {t("Custom template unavailable; the simple page will be used.")}
            </p>
          )}
          <details className="text-xs">
            <summary className="cursor-pointer">
              {t("Recent subscription requests")}
            </summary>
            <ul className="mt-3 space-y-2">
              {history.map((row, index) => (
                <li
                  key={`${row.at}-${index}`}
                  className="flex flex-wrap justify-between gap-2"
                >
                  <time dateTime={row.at}>
                    {new Date(row.at).toLocaleString()}
                  </time>
                  <span>
                    {row.client_app} · {row.format} · {row.status}
                  </span>
                </li>
              ))}
              {history.length === 0 && <li>{t("No records yet")}</li>}
            </ul>
          </details>
          {result.checks.some((check) => check.probe_available) &&
            can(session, "clients.write") && (
              <div className="space-y-2 text-xs">
                <p className="leading-6 text-[var(--muted)]">
                  {t(
                    "Connection tests use the selected node and the real client credentials. Test traffic counts toward quota and can start the first-use duration.",
                  )}
                </p>
                <label className="label">
                  {t("Test destination")}
                  <Input
                    type="url"
                    value={target}
                    onChange={(e) => setTarget(e.target.value)}
                    dir="ltr"
                  />
                </label>
              </div>
            )}
          {result.checks.map((check, index) => (
            <article
              key={`${check.inbound}-${index}`}
              className="rounded-lg border border-[var(--border)] p-4"
            >
              <h3 className="text-sm font-medium">
                {check.name}{" "}
                <span className="text-xs text-[var(--muted)]">
                  {check.protocol}
                </span>
              </h3>
              {check.reasons.map((reason) => (
                <p key={reason} className="mt-2 text-xs text-amber-600">
                  {t(reason)}
                </p>
              ))}
              {check.probe_available && can(session, "clients.write") && (
                <div className="mt-3 space-y-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={!!probeBusy || !check.eligible}
                    onClick={() => probe(check.inbound, check.host_index ?? 0)}
                  >
                    {probeBusy === `${check.inbound}:${check.host_index ?? 0}`
                      ? t("Testing connection…")
                      : t("Test connection")}
                  </Button>
                  {probes[`${check.inbound}:${check.host_index ?? 0}`] && (
                    <p role="status" className="text-xs">
                      {probes[`${check.inbound}:${check.host_index ?? 0}`].ok
                        ? t("Connection successful")
                        : t("Connection failed")}{" "}
                      ·{" "}
                      {probes[`${check.inbound}:${check.host_index ?? 0}`]
                        .latency_ms ?? "—"}{" "}
                      ms{" "}
                      {probes[`${check.inbound}:${check.host_index ?? 0}`]
                        .status_code || ""}{" "}
                      {probes[`${check.inbound}:${check.host_index ?? 0}`]
                        .error || ""}
                    </p>
                  )}
                </div>
              )}
              {check.probe_available && check.protocol === "amneziawg" && (
                <p className="mt-2 text-xs leading-6 text-[var(--muted)]">
                  {t(
                    "AmneziaWG testing can interrupt this peer's active connections. Use a separate test account.",
                  )}
                </p>
              )}
              <dl className="mt-3 space-y-2">
                {Object.entries(check.formats).map(([format, value]) => (
                  <div
                    key={format}
                    className="flex flex-wrap justify-between gap-2 text-xs"
                  >
                    <dt>{format}</dt>
                    <dd className="max-w-full text-[var(--muted)]">
                      {value.available
                        ? t("Available")
                        : value.reason || t("Unavailable")}
                    </dd>
                  </div>
                ))}
              </dl>
            </article>
          ))}
        </div>
      )}
    </Dialog>
  );
}
