import { useState } from "react";
import { api, type InboundItem, type SubscriptionHost } from "@/lib/api";
import { t } from "@/lib/i18n";
import { Input, Select } from "@/components/ui/controls";
import { Button } from "@/components/ui/button";

export function SubscriptionHosts({
  hosts,
  onChange,
  disabled,
}: {
  hosts: SubscriptionHost[];
  onChange: (hosts: SubscriptionHost[]) => void;
  disabled: boolean;
}) {
  const [inbounds, setInbounds] = useState<InboundItem[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  async function load() {
    if (inbounds.length || loading) return;
    setLoading(true);
    try {
      setInbounds(await api<InboundItem[]>("/inbounds/?limit=500"));
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : t("Could not load inbounds"));
    } finally {
      setLoading(false);
    }
  }
  function change(
    index: number,
    key: keyof SubscriptionHost,
    value: string | number | boolean,
  ) {
    onChange(
      hosts.map((host, i) => (i === index ? { ...host, [key]: value } : host)),
    );
  }
  return (
    <details
      className="rounded-lg border border-[var(--border)] p-4"
      onToggle={(e) => {
        if (e.currentTarget.open) void load();
      }}
    >
      <summary className="cursor-pointer text-sm font-medium">
        {t("Subscription hosts")} ({hosts.length})
      </summary>
      <p className="my-4 text-xs leading-6 text-[var(--muted)]">
        {t(
          "Publish an inbound through several addresses without changing its listener. A zero port uses the inbound port. If all hosts for an inbound are disabled, it is hidden from subscriptions.",
        )}
      </p>
      {error && (
        <p role="alert" className="my-3 text-xs">
          {error}
          <Button variant="ghost" size="sm" onClick={() => void load()}>
            {t("Retry")}
          </Button>
        </p>
      )}
      <div className="space-y-4">
        {hosts.map((host, index) => (
          <fieldset
            key={index}
            disabled={disabled}
            className="grid gap-3 rounded-md border border-[var(--border)] p-3 sm:grid-cols-2"
          >
            <legend className="px-1 text-xs">
              {host.name || t("New host")}
            </legend>
            <label className="label">
              {t("Name")}
              <Input
                className="input"
                value={host.name}
                maxLength={96}
                onChange={(e) => change(index, "name", e.target.value)}
              />
            </label>
            <label className="label">
              {t("Inbound")}
              <Select
                className="input"
                value={host.inbound}
                onChange={(e) =>
                  change(index, "inbound", Number(e.target.value))
                }
              >
                <option value={0}>{t("Select inbound")}</option>
                {inbounds.map((inbound) => (
                  <option key={inbound.id} value={inbound.id}>
                    {inbound.name} · {inbound.node_name}
                  </option>
                ))}
              </Select>
            </label>
            <label className="label">
              {t("Public address")}
              <Input
                className="input"
                dir="ltr"
                value={host.address}
                onChange={(e) => change(index, "address", e.target.value)}
              />
            </label>
            <label className="label">
              {t("Public port")}
              <Input
                className="input"
                type="number"
                min={0}
                max={65535}
                value={host.port}
                onChange={(e) => change(index, "port", Number(e.target.value))}
              />
            </label>
            {(
              [
                ["sni", "SNI"],
                ["path", "Path"],
                ["host_header", "Host header"],
              ] as const
            ).map(([key, label]) => (
              <label key={key} className="label">
                {t(label)}
                <Input
                  className="input"
                  dir="ltr"
                  value={host[key]}
                  onChange={(e) => change(index, key, e.target.value)}
                />
              </label>
            ))}
            <div className="flex items-center justify-between gap-3">
              <label className="flex items-center gap-2 text-xs">
                <Input
                  type="checkbox"
                  checked={host.enabled}
                  onChange={(e) => change(index, "enabled", e.target.checked)}
                />
                {t("Enabled")}
              </label>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onChange(hosts.filter((_, i) => i !== index))}
              >
                {t("Remove host")}
              </Button>
            </div>
          </fieldset>
        ))}
      </div>
      <Button
        variant="outline"
        size="sm"
        className="mt-4"
        disabled={disabled || hosts.length >= 200}
        onClick={() => {
          void load();
          onChange([
            ...hosts,
            {
              name: "",
              inbound: 0,
              address: "",
              port: 0,
              sni: "",
              path: "",
              host_header: "",
              enabled: true,
            },
          ]);
        }}
      >
        {t("Add host")}
      </Button>
    </details>
  );
}
