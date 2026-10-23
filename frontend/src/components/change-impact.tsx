import { t } from "@/lib/i18n";
export type ChangeImpact = {
  revision: string;
  customers: number;
  nodes: number[];
  connections_added: number;
  connections_removed: number;
  connections_preserved: number;
};
export function ImpactSummary({ impact }: { impact: ChangeImpact }) {
  return (
    <section
      aria-label={t("Change impact", "اثر تغییرات")}
      className="space-y-3 rounded-lg border border-emerald-500/20 bg-emerald-500/5 p-4 text-[12px]"
    >
      <h4 className="font-semibold">{t("Change impact", "اثر تغییرات")}</h4>
      <dl className="grid grid-cols-2 gap-3">
        {[
          [t("Customers"), impact.customers],
          [t("Nodes"), impact.nodes.length],
          [t("Connections added", "اتصال‌های جدید"), impact.connections_added],
          [
            t("Connections removed", "اتصال‌های حذف‌شونده"),
            impact.connections_removed,
          ],
        ].map(([label, value]) => (
          <div key={label}>
            <dt className="text-[var(--muted)]">{label}</dt>
            <dd className="mt-1 font-semibold">{value}</dd>
          </div>
        ))}
      </dl>
      <p className="text-[11px] text-[var(--muted)]">
        {t(
          "Existing credentials and usage are preserved. Plan defaults are not reapplied.",
          "اطلاعات اتصال و مصرف قبلی حفظ می‌شوند. مقادیر پیش‌فرض پلن دوباره اعمال نمی‌شوند.",
        )}
      </p>
    </section>
  );
}
