import { useState, type FormEvent } from "react";
import { t, useLocale } from "@/lib/i18n";
import { datePreferences, formatDate, saveDatePreferences } from "@/lib/dates";
import { Input, Select } from "@/components/ui/controls";
import { Button } from "@/components/ui/button";
export function DatePreferences() {
  useLocale();
  const [value, setValue] = useState(datePreferences),
    [error, setError] = useState(""),
    [saved, setSaved] = useState(false);
  function save(e?: FormEvent) {
    e?.preventDefault();
    try {
      saveDatePreferences(value.calendar, value.timezone);
      setError("");
      setSaved(true);
    } catch {
      setSaved(false);
      setError(t("Enter a valid IANA timezone."));
    }
  }
  return (
    <section className="space-y-3 rounded-lg border border-[var(--border)] p-4">
      <h3 className="text-[13px] font-medium">{t("Date display")}</h3>
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="label">
          {t("Calendar")}
          <Select
            value={value.calendar}
            onChange={(e) => {
              setSaved(false);
              setValue({ ...value, calendar: e.target.value });
            }}
          >
            <option value="auto">{t("Language default")}</option>
            <option value="persian">{t("Persian (Jalali)")}</option>
            <option value="gregory">{t("Gregorian")}</option>
          </Select>
        </label>
        <label className="label">
          {t("Display timezone")}
          <Input
            required
            value={value.timezone}
            placeholder="Asia/Tehran"
            onChange={(e) => {
              setSaved(false);
              setValue({ ...value, timezone: e.target.value });
            }}
          />
        </label>
      </div>
      <p className="text-[11px] text-[var(--muted)]">
        {t(
          "Date inputs keep their labeled timezone and Gregorian format. Display preferences are saved for this browser.",
        )}
      </p>
      {saved && (
        <p role="status">
          {t("Saved")} ·{" "}
          {formatDate(new Date(), { hour: "2-digit", minute: "2-digit" })}
        </p>
      )}
      {error && (
        <p role="alert" className="text-red-500">
          {error}
        </p>
      )}
      <Button type="button" variant="outline" onClick={() => save()}>
        {t("Save date preferences")}
      </Button>
    </section>
  );
}
