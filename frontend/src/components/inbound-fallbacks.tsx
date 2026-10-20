import { t, useLocale } from "@/lib/i18n";
import { Input, Select } from "@/components/ui/controls";
import { Button } from "@/components/ui/button";

export type FallbackEntry = {
  name?: string;
  alpn?: string;
  path?: string;
  dest: string | number;
  xver?: number;
};
export function InboundFallbacks({
  value,
  onChange,
}: {
  value: FallbackEntry[];
  onChange: (next: FallbackEntry[]) => void;
}) {
  useLocale();
  const update = (index: number, patch: Partial<FallbackEntry>) =>
    onChange(value.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  return (
    <section
      className="rounded-lg border border-[var(--border)] p-3 space-y-3"
      aria-label={t("Fallback destinations")}
    >
      <h3 className="text-sm font-medium">{t("Fallback destinations")}</h3>
      <p className="text-xs text-[var(--muted)]">
        {t(
          "Unauthenticated traffic is forwarded by Xray. Configure a web server destination; PROXY protocol requires support at that destination.",
        )}
      </p>
      <p className="text-xs text-[var(--muted)]">
        {t(
          "Each SNI/ALPN group needs an empty-path default. Keep one entry with empty SNI, ALPN and path as the overall default.",
        )}
      </p>
      {value.map((row, index) => (
        <fieldset
          key={index}
          className="grid gap-3 sm:grid-cols-2 border border-[var(--border)] rounded-lg p-3"
        >
          <legend className="text-xs px-1">
            {t("Fallback")} {index + 1}
          </legend>
          <label className="label">
            {t("Destination")}
            <Input
              className="input"
              dir="ltr"
              required
              placeholder="127.0.0.1:8080"
              value={row.dest}
              onChange={(e) => update(index, { dest: e.target.value })}
            />
          </label>
          <label className="label">
            SNI
            <Input
              className="input"
              dir="ltr"
              maxLength={255}
              value={row.name || ""}
              onChange={(e) => update(index, { name: e.target.value })}
            />
          </label>
          <label className="label">
            ALPN
            <Input
              className="input"
              dir="ltr"
              maxLength={255}
              placeholder="http/1.1"
              value={row.alpn || ""}
              onChange={(e) => update(index, { alpn: e.target.value })}
            />
          </label>
          <label className="label">
            {t("Path")}
            <Input
              className="input"
              dir="ltr"
              maxLength={255}
              placeholder="/"
              value={row.path || ""}
              onChange={(e) => update(index, { path: e.target.value })}
            />
          </label>
          <label className="label">
            {t("PROXY protocol")}
            <Select
              className="input"
              value={String(row.xver || 0)}
              onChange={(e) => update(index, { xver: Number(e.target.value) })}
            >
              <option value="0">{t("Off")}</option>
              <option value="1">v1</option>
              <option value="2">v2</option>
            </Select>
          </label>
          <Button
            type="button"
            variant="outline"
            onClick={() => onChange(value.filter((_, i) => i !== index))}
          >
            {t("Remove fallback")}
          </Button>
        </fieldset>
      ))}
      <Button
        type="button"
        variant="outline"
        disabled={value.length >= 32}
        onClick={() => onChange([...value, { dest: "", xver: 0 }])}
      >
        {t("Add fallback")}
      </Button>
    </section>
  );
}
