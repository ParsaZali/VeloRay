import { SubscriptionHosts } from "@/components/subscription-hosts";
import { useState } from "react";
import { api, type SubscriptionHost, type AppSettings } from "@/lib/api";
import { t } from "@/lib/i18n";
import { Input, Textarea } from "@/components/ui/controls";
import { Button } from "@/components/ui/button";

type Key =
  | "subscription_hosts"
  | "subscription_template_path"
  | "subscription_hwid_required"
  | "subscription_hwid_limit"
  | "subscription_clash_template"
  | "subscription_singbox_template"
  | "subscription_xray_template";
export function SubscriptionOptions({
  cfg,
  set,
  disabled,
}: {
  cfg: AppSettings;
  set: (
    key: Key,
    value: string | number | boolean | SubscriptionHost[],
  ) => void;
  disabled: boolean;
}) {
  const [status, setStatus] = useState("");
  const [checking, setChecking] = useState(false);
  async function validate() {
    setChecking(true);
    setStatus("");
    try {
      await api("/subscription-template/validate", {
        method: "POST",
        body: JSON.stringify({ path: cfg.subscription_template_path }),
      });
      setStatus(t("Template is valid. Save settings to activate it."));
    } catch (e) {
      setStatus(
        e instanceof Error ? e.message : t("Template validation failed"),
      );
    } finally {
      setChecking(false);
    }
  }
  return (
    <div className="space-y-4 sm:col-span-2">
      {cfg.subscription_template === "custom" && (
        <div className="space-y-3 rounded-lg border border-[var(--border)] p-4">
          <label className="label" htmlFor="subscription-template-path">
            {t("HTML template file")}
          </label>
          <div className="flex flex-wrap gap-2">
            <Input
              id="subscription-template-path"
              className="input min-w-0 flex-1"
              dir="ltr"
              value={cfg.subscription_template_path}
              disabled={disabled}
              onChange={(e) => {
                set("subscription_template_path", e.target.value);
                setStatus("");
              }}
              placeholder="/etc/veloray/templates/custom/index.html"
            />
            <Button
              type="button"
              variant="outline"
              disabled={disabled || checking || !cfg.subscription_template_path}
              onClick={() => void validate()}
            >
              {t("Validate template")}
            </Button>
          </div>
          <p className="text-xs leading-6 text-[var(--muted)]">
            {t(
              "Use an HTML file inside /etc/veloray/templates. Changes reload automatically; an invalid file falls back to the simple page.",
            )}
          </p>
          <p className="text-xs leading-6 text-[var(--muted)]" dir="ltr">
            {"{{.Name}} · {{.URL}} · {{.Used}} · {{.Expiry}} · {{.AssetsURL}}"}
          </p>
          {status && (
            <p role="status" className="text-xs">
              {status}
            </p>
          )}
        </div>
      )}
      <SubscriptionHosts
        hosts={cfg.subscription_hosts}
        onChange={(hosts) => set("subscription_hosts", hosts)}
        disabled={disabled}
      />
      <details className="rounded-lg border border-[var(--border)] p-4">
        <summary className="cursor-pointer text-sm font-medium">
          {t("Subscription device policy")}
        </summary>
        <div className="mt-4 grid gap-4 sm:grid-cols-2">
          <label className="label">
            {t("Default HWID device limit")}
            <Input
              className="input"
              type="number"
              min={0}
              max={1000}
              value={cfg.subscription_hwid_limit}
              disabled={disabled}
              onChange={(e) =>
                set("subscription_hwid_limit", Number(e.target.value))
              }
            />
            <span className="field-hint">
              {t("0 = unlimited. A positive limit requires HWID support.")}
            </span>
          </label>
          <label className="flex items-start gap-3 text-sm">
            <Input
              type="checkbox"
              checked={cfg.subscription_hwid_required}
              disabled={disabled}
              onChange={(e) =>
                set("subscription_hwid_required", e.target.checked)
              }
            />
            <span>
              {t("Require HWID even without a device cap")}
              <small className="mt-2 block text-xs leading-6 text-[var(--muted)]">
                {t(
                  "Apps without HWID cannot download configurations when enabled. Existing copied configurations are not revoked.",
                )}
              </small>
            </span>
          </label>
        </div>
      </details>
      <details className="rounded-lg border border-[var(--border)] p-4">
        <summary className="cursor-pointer text-sm font-medium">
          {t("Client configuration templates")}
        </summary>
        <p className="my-4 text-xs leading-6 text-[var(--muted)]">
          {t(
            "Optional JSON objects. Leave blank for built-in output. Generated proxies and outbounds are inserted automatically. Use $VELO_PROXIES inside a Clash group's proxies array.",
          )}
        </p>
        <div className="grid gap-4">
          <label className="label">
            Xray JSON
            <Textarea
              className="input min-h-32 font-mono"
              dir="ltr"
              disabled={disabled}
              value={cfg.subscription_xray_template}
              onChange={(e) =>
                set("subscription_xray_template", e.target.value)
              }
              placeholder={
                '{"inbounds":[{"listen":"127.0.0.1","port":10808,"protocol":"socks","settings":{"udp":true}}]}'
              }
            />
          </label>
          <label className="label">
            Clash / Mihomo
            <Textarea
              className="input min-h-32 font-mono"
              dir="ltr"
              disabled={disabled}
              value={cfg.subscription_clash_template}
              onChange={(e) =>
                set("subscription_clash_template", e.target.value)
              }
              placeholder={
                '{"dns":{"enable":true,"nameserver":["https://1.1.1.1/dns-query"]}}'
              }
            />
          </label>
          <label className="label">
            sing-box
            <Textarea
              className="input min-h-32 font-mono"
              dir="ltr"
              disabled={disabled}
              value={cfg.subscription_singbox_template}
              onChange={(e) =>
                set("subscription_singbox_template", e.target.value)
              }
              placeholder={
                '{"dns":{"servers":[{"type":"https","server":"1.1.1.1"}]}}'
              }
            />
          </label>
        </div>
      </details>
    </div>
  );
}
