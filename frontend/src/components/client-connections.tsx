import { useEffect, useState } from "react";
import {
  Globe,
  MonitorSmartphone,
  RefreshCw,
  X,
  Ban,
  RotateCcw,
  Trash2,
} from "lucide-react";
import { Dialog } from "@/components/common/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  api,
  type ClientItem,
  type ClientIPList,
  type ClientDeviceList,
} from "@/lib/api";
import { useSession } from "@/lib/session";
import { localeName, t, useLocale } from "@/lib/i18n";

const date = (value: string) => {
  const time = new Date(value);
  return Number.isNaN(time.getTime()) ? "—" : time.toLocaleString(localeName());
};
export function ClientConnections({
  client,
  kind,
  onClose,
}: {
  client: ClientItem;
  kind: "ips" | "devices";
  onClose: () => void;
}) {
  useLocale();
  const session = useSession();
  const canManage = Boolean(
    session?.is_superuser || session?.permissions?.includes("clients.write"),
  );
  const [pendingDevice, setPendingDevice] = useState<number | null>(null);
  async function deviceAction(
    id: number,
    action: "revoke" | "allow" | "forget",
  ) {
    if (pendingDevice !== null) return;
    setPendingDevice(id);
    try {
      await api(`/clients/${client.id}/device-action/`, {
        method: "POST",
        body: JSON.stringify({ device_id: id, action }),
      });
      setRevision((v) => v + 1);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : t("Could not update device"));
    } finally {
      setPendingDevice(null);
    }
  }
  const [data, setData] = useState<ClientIPList | ClientDeviceList | null>(
    null,
  );
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true,
      running = false;
    const controller = new AbortController();
    const load = async () => {
      if (running || document.hidden) return;
      running = true;
      try {
        const value = await api<ClientIPList | ClientDeviceList>(
          `/clients/${client.id}/${kind}/`,
          { signal: controller.signal },
        );
        if (active) {
          setData(value);
          setError("");
        }
      } catch (e) {
        if (active && !controller.signal.aborted) {
          setData(null);
          setError(
            e instanceof Error ? e.message : t("Could not load connections"),
          );
        }
      } finally {
        running = false;
        if (active) setLoading(false);
      }
    };
    setLoading(true);
    void load();
    const interval = setInterval(load, 5000);
    document.addEventListener("visibilitychange", load);
    return () => {
      active = false;
      controller.abort();
      clearInterval(interval);
      document.removeEventListener("visibilitychange", load);
    };
  }, [client.id, kind, revision]);
  const title = kind === "ips" ? t("IP's") : t("Device's");
  return (
    <Dialog
      label={`${title} · ${client.name}`}
      onClose={onClose}
      padded={false}
    >
      <header className="flex items-center justify-between gap-3 border-b border-[var(--border)] p-5">
        <div className="flex min-w-0 items-center gap-3">
          {kind === "ips" ? (
            <Globe size={20} />
          ) : (
            <MonitorSmartphone size={20} />
          )}
          <div>
            <h2 className="text-base font-semibold">{title}</h2>
            <p className="break-all text-xs text-[var(--muted)]">
              {client.name}
            </p>
          </div>
          {data && <Badge tone="neutral">{data.count}</Badge>}
        </div>
        <div className="flex gap-1">
          <Button
            variant="ghost"
            size="icon"
            aria-label={t("Refresh")}
            aria-disabled={loading}
            onClick={() => {
              if (!loading) setRevision((v) => v + 1);
            }}
          >
            <RefreshCw size={16} />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t("Close")}
            onClick={onClose}
          >
            <X size={18} />
          </Button>
        </div>
      </header>
      <div
        className="max-h-[65vh] space-y-3 overflow-y-auto p-5"
        aria-busy={loading}
      >
        {loading && !data && !error && (
          <p role="status" className="text-sm text-[var(--muted)]">
            {t("Loading…")}
          </p>
        )}
        {error && (
          <p role="alert" className="text-sm text-[var(--danger)]">
            {error}
          </p>
        )}
        {data && kind === "ips" && (
          <>
            {!(data as ClientIPList).available && (
              <p role="status" className="text-xs text-[var(--muted)]">
                {t("Some connection data is unavailable")}
              </p>
            )}
            {(data as ClientIPList).items.map((item) => (
              <article
                key={item.ip}
                className="rounded-xl border border-[var(--border)] bg-[var(--surface-2)] p-4"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <code dir="ltr" className="break-all text-sm">
                    {item.ip}
                  </code>
                  <Badge tone="green">{t("Online")}</Badge>
                </div>
                <p className="mt-2 text-xs text-[var(--muted)]">
                  {item.connections
                    .map((c) => `${c.node_name} · ${c.inbound_name}`)
                    .join(" / ")}
                </p>
                <p className="mt-2 text-xs text-[var(--muted)]">
                  {t("Last observed")} · {date(item.last_seen_at)}
                </p>
              </article>
            ))}
            {data.count === 0 && (data as ClientIPList).available && (
              <p className="py-8 text-center text-sm text-[var(--muted)]">
                {t("No connected IPs")}
              </p>
            )}
          </>
        )}
        {data && kind === "devices" && (
          <>
            {(data as ClientDeviceList).items.map((item) => (
              <article
                key={item.id}
                className="rounded-xl border border-[var(--border)] bg-[var(--surface-2)] p-4"
              >
                <div className="flex items-start gap-3">
                  <MonitorSmartphone
                    size={20}
                    className="shrink-0 text-[var(--muted)]"
                  />
                  <div className="min-w-0 flex-1">
                    <h3 className="break-words text-sm font-semibold">
                      {item.device_model || t("Device")}{" "}
                      {item.revoked && (
                        <Badge tone="neutral">{t("Revoked")}</Badge>
                      )}
                    </h3>
                    {(item.device_os || item.os_version) && (
                      <p className="mt-1 text-xs text-[var(--muted)]">
                        {[item.device_os, item.os_version]
                          .filter(Boolean)
                          .join(" · ")}
                      </p>
                    )}
                    <dl className="mt-3 space-y-2 text-xs">
                      <div>
                        <dt className="text-[var(--muted)]">HWID</dt>
                        <dd dir="ltr" className="break-all font-mono">
                          {item.hwid}
                        </dd>
                      </div>
                      {item.user_agent && (
                        <div>
                          <dt className="text-[var(--muted)]">
                            {t("Application")}
                          </dt>
                          <dd dir="ltr" className="break-all">
                            {item.user_agent}
                          </dd>
                        </div>
                      )}
                      <div>
                        <dt className="text-[var(--muted)]">
                          {t("First registered")}
                        </dt>
                        <dd>{date(item.first_seen_at)}</dd>
                      </div>
                      <div>
                        <dt className="text-[var(--muted)]">
                          {t("Last subscription update")}
                        </dt>
                        <dd>{date(item.last_seen_at)}</dd>
                      </div>
                    </dl>
                    {canManage && (
                      <div className="mt-4 flex gap-2">
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={pendingDevice !== null}
                          onClick={() =>
                            void deviceAction(
                              item.id,
                              item.revoked ? "allow" : "revoke",
                            )
                          }
                        >
                          {item.revoked ? (
                            <RotateCcw size={14} />
                          ) : (
                            <Ban size={14} />
                          )}
                          {item.revoked
                            ? t("Allow device")
                            : t("Revoke device")}
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={pendingDevice !== null}
                          onClick={() => void deviceAction(item.id, "forget")}
                        >
                          <Trash2 size={14} />
                          {t("Forget device")}
                        </Button>
                      </div>
                    )}
                  </div>
                </div>
              </article>
            ))}
            {data.count === 0 && (
              <p className="py-8 text-center text-sm text-[var(--muted)]">
                {t("No registered devices")}
              </p>
            )}
            <p className="text-xs text-[var(--muted)]">
              {t("Devices are registered by apps that send HWID.")}
            </p>
          </>
        )}
      </div>
    </Dialog>
  );
}
