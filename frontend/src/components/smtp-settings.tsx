import { t, useLocale } from "@/lib/i18n";
import type { AppSettings } from "@/lib/api";
import { Input, Select } from "@/components/ui/controls";
export function SMTPSettings({
  value,
  onChange,
  password,
  onPassword,
  clear,
  onClear,
}: {
  value: AppSettings;
  onChange: (key: keyof AppSettings, value: any) => void;
  password: string;
  onPassword: (v: string) => void;
  clear: boolean;
  onClear: (v: boolean) => void;
}) {
  useLocale();
  return (
    <section className="space-y-4 border-t border-[var(--border)] pt-5">
      <h3 className="text-[13px] font-medium">
        {t("Email delivery and lifecycle alerts")}
      </h3>
      <label className="flex gap-2">
        <Input
          type="checkbox"
          checked={value.smtp_enabled}
          onChange={(e) => onChange("smtp_enabled", e.target.checked)}
        />
        {t("Enable SMTP email")}
      </label>
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="label">
          {t("SMTP hostname")}
          <Input
            value={value.smtp_host}
            onChange={(e) => onChange("smtp_host", e.target.value)}
            placeholder="smtp.example.com"
          />
        </label>
        <label className="label">
          {t("SMTP port")}
          <Input
            type="number"
            min={1}
            max={65535}
            value={value.smtp_port}
            onChange={(e) => onChange("smtp_port", Number(e.target.value))}
          />
        </label>
        <label className="label">
          {t("Encryption")}
          <Select
            value={value.smtp_tls_mode}
            onChange={(e) => onChange("smtp_tls_mode", e.target.value)}
          >
            <option value="tls">TLS</option>
            <option value="starttls">STARTTLS</option>
          </Select>
        </label>
        <label className="label">
          {t("Sender email")}
          <Input
            type="email"
            value={value.smtp_from}
            onChange={(e) => onChange("smtp_from", e.target.value)}
          />
        </label>
        <label className="label">
          {t("SMTP username")}
          <Input
            autoComplete="off"
            value={value.smtp_username}
            onChange={(e) => onChange("smtp_username", e.target.value)}
          />
        </label>
        <label className="label">
          {t("SMTP password")}
          <Input
            type="password"
            autoComplete="new-password"
            value={password}
            maxLength={4096}
            onChange={(e) => onPassword(e.target.value)}
            placeholder={
              value.smtp_configured
                ? t("Leave blank to keep the stored secret")
                : ""
            }
          />
        </label>
      </div>
      {value.smtp_configured && (
        <label className="flex gap-2 text-[12px]">
          <Input
            type="checkbox"
            checked={clear}
            onChange={(e) => onClear(e.target.checked)}
          />
          {t("Clear SMTP password")}
        </label>
      )}
      <p className="text-[11px] text-[var(--muted)]">
        {t(
          "Only verified TLS is allowed. Save settings, then add an Email destination below. Passwords are encrypted and never returned.",
        )}
      </p>
      <div className="grid gap-3 sm:grid-cols-3">
        {(
          [
            ["alert_quota_percent", "Quota warning (%)", 99],
            ["alert_expiry_days", "Expiry warning (days)", 365],
            ["alert_inactive_days", "Inactive warning (days)", 365],
          ] as const
        ).map(([key, label, max]) => (
          <label className="label" key={key}>
            {t(label)}
            <Input
              type="number"
              min={0}
              max={max}
              value={value[key]}
              onChange={(e) => onChange(key, Number(e.target.value))}
            />
          </label>
        ))}
      </div>
      <p className="text-[11px] text-[var(--muted)]">
        {t(
          "Zero disables an alert. Events are deduplicated per shared account and delivered through matching notification destinations.",
        )}
      </p>
    </section>
  );
}
