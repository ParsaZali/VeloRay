import { useEffect, useRef, useState } from "react";
import {
  Check,
  Copy,
  ExternalLink,
  LoaderCircle,
  Network,
  RefreshCw,
} from "lucide-react";
import { api } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/controls";

type Endpoint = {
  managed: boolean;
  panel_origin: string;
  subscription_origin: string;
  port: number;
  applied_port: number;
  revision: string;
  status: "unconfigured" | "pending" | "ready" | "error";
  error: string;
};

export function SubscriptionEndpoint({ enabled }: { enabled: boolean }) {
  useLocale();
  const [endpoint, setEndpoint] = useState<Endpoint | null>(null);
  const [port, setPort] = useState("");
  const [revision, setRevision] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [feedback, setFeedback] = useState("");
  const applying = useRef<AbortController | null>(null);
  useEffect(() => () => applying.current?.abort(), []);
  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let initial = true;
    async function load() {
      try {
        const next = await api<Endpoint>("/subscription-endpoint", {
          signal: controller.signal,
        });
        if (controller.signal.aborted) return;
        setEndpoint(next);
        if (initial) {
          setPort(String(next.port || 8444));
          initial = false;
        }
        setError("");
        if (next.status === "pending")
          timer = setTimeout(() => void load(), 2000);
      } catch (e) {
        if (!controller.signal.aborted)
          setError(
            e instanceof Error
              ? e.message
              : t(
                  "Endpoint status unavailable",
                  "وضعیت پورت اشتراک در دسترس نیست",
                ),
          );
      }
    }
    void load();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [enabled, revision]);
  async function apply() {
    if (!endpoint || busy) return;
    const value = Number(port);
    if (!Number.isInteger(value) || value < 1 || value > 65535) {
      setError(
        t(
          "Choose a port between 1 and 65535.",
          "پورتی بین ۱ تا ۶۵۵۳۵ انتخاب کنید.",
        ),
      );
      return;
    }
    const controller = new AbortController();
    applying.current = controller;
    setBusy(true);
    setError("");
    setFeedback("");
    try {
      const next = await api<Endpoint>("/subscription-endpoint", {
        method: "PUT",
        signal: controller.signal,
        body: JSON.stringify({ port: value, revision: endpoint.revision }),
      });
      if (!controller.signal.aborted) {
        setEndpoint(next);
        setRevision((value) => value + 1);
      }
    } catch (e) {
      if (!controller.signal.aborted)
        setError(
          e instanceof Error
            ? t(e.message)
            : t("Could not apply subscription port", "پورت اشتراک اعمال نشد"),
        );
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  async function copy() {
    if (!endpoint) return;
    try {
      await navigator.clipboard.writeText(endpoint.subscription_origin);
      setFeedback(t("Address copied", "آدرس کپی شد"));
    } catch {
      setFeedback(
        t("Select and copy the address", "آدرس را انتخاب و دستی کپی کنید"),
      );
    }
  }
  if (!enabled) return null;
  const pending = endpoint?.status === "pending",
    ready = endpoint?.status === "ready";
  return (
    <section
      className="subscription-endpoint sm:col-span-2"
      aria-labelledby="subscription-endpoint-title"
    >
      <div className="endpoint-heading">
        <span className="endpoint-icon">
          <Network size={19} aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 id="subscription-endpoint-title">
            {t("Separate subscription port", "پورت جداگانهٔ اشتراک")}
          </h3>
          <p>
            {t(
              "Keep customer subscriptions separate from panel administration.",
              "لینک‌های اشتراک مشتریان را از بخش مدیریت پنل جدا کنید.",
            )}
          </p>
        </div>
        <span
          role="status"
          className={`endpoint-status ${ready ? "is-ready" : pending ? "is-pending" : ""}`}
        >
          {ready ? (
            <Check size={13} aria-hidden="true" />
          ) : pending ? (
            <LoaderCircle
              size={13}
              className="animate-spin"
              aria-hidden="true"
            />
          ) : null}
          {ready
            ? t("Active", "فعال")
            : pending
              ? t("Applying", "در حال اعمال")
              : endpoint?.status === "error"
                ? t("Needs attention", "نیازمند بررسی")
                : t("Not configured", "تنظیم نشده")}
        </span>
      </div>
      <div className="endpoint-addresses">
        <div>
          <span>{t("Panel address", "آدرس پنل")}</span>
          <code dir="ltr">{endpoint?.panel_origin || "—"}</code>
        </div>
        <div className="endpoint-public">
          <span>{t("Subscription address", "آدرس اشتراک")}</span>
          <code dir="ltr">
            {endpoint?.applied_port ? endpoint.subscription_origin : "—"}
          </code>
        </div>
      </div>
      <div className="endpoint-controls">
        <label className="label" htmlFor="subscription-public-port">
          {t("Subscription port", "پورت اشتراک")}
          <Input
            id="subscription-public-port"
            className="input"
            inputMode="numeric"
            type="number"
            min={1}
            max={65535}
            value={port}
            disabled={!endpoint?.managed || busy || pending}
            onChange={(e) => setPort(e.target.value)}
            dir="ltr"
            aria-describedby="subscription-port-help"
          />
        </label>
        <Button
          type="button"
          disabled={!endpoint?.managed || busy || pending || !port}
          onClick={() => void apply()}
        >
          {busy || pending ? (
            <LoaderCircle
              size={15}
              className="animate-spin"
              aria-hidden="true"
            />
          ) : null}
          {pending
            ? t("Applying port…", "در حال اعمال پورت…")
            : t("Apply port", "اعمال پورت")}
        </Button>
        <Button
          type="button"
          variant="outline"
          aria-label={t("Refresh endpoint status", "تازه‌سازی وضعیت پورت")}
          onClick={() => setRevision((value) => value + 1)}
          disabled={busy}
        >
          <RefreshCw size={15} aria-hidden="true" />
        </Button>
        {Boolean(endpoint?.applied_port) && (
          <Button type="button" variant="ghost" onClick={() => void copy()}>
            <Copy size={14} aria-hidden="true" />
            {t("Copy address", "کپی آدرس")}
          </Button>
        )}
      </div>
      <p className="endpoint-help" id="subscription-port-help">
        {t(
          "Changing the port changes subscription URLs. Refresh customer links after applying a new port.",
          "با تغییر پورت، آدرس اشتراک تغییر می‌کند. پس از اعمال، لینک تازه را برای مشتریان بفرستید.",
        )}
      </p>
      <p className="endpoint-help">
        {t(
          "Allow this TCP port in your server and provider firewalls.",
          "این پورت TCP را در فایروال سرور و ارائه‌دهنده باز کنید.",
        )}
      </p>
      {endpoint && !endpoint.managed ? (
        <p className="endpoint-help">
          {t(
            "Managed port changes are available after installing the native or Docker subscription service.",
            "تنظیم پورت پس از نصب سرویس اشتراک روی سرور یا Docker در دسترس است.",
          )}
        </p>
      ) : null}
      {pending ? (
        <p className="endpoint-help">
          {t(
            "The previous address stays active until the new listener is ready.",
            "تا زمان آماده شدن پورت جدید، آدرس قبلی فعال می‌ماند.",
          )}
        </p>
      ) : null}
      {error || endpoint?.error ? (
        <p role="alert" className="endpoint-error">
          {error || t(endpoint?.error || "")}
        </p>
      ) : null}
      {feedback ? (
        <p role="status" className="endpoint-help">
          {feedback}
        </p>
      ) : null}
      {ready ? (
        <a
          className="endpoint-health"
          href={`${endpoint.subscription_origin}/api/health`}
          target="_blank"
          rel="noopener noreferrer"
        >
          {t("Check subscription service", "بررسی سرویس اشتراک")}
          <ExternalLink size={12} aria-hidden="true" />
        </a>
      ) : null}
    </section>
  );
}
