import { useLocale, t } from "@/lib/i18n";
import type { ReactNode } from "react";
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  useLocale();
  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <h2 className="text-[22px] font-semibold tracking-[-.025em] text-[var(--fg)]">
          {t(title)}
        </h2>
        {description && (
          <p className="mt-1 max-w-3xl text-[13px] leading-relaxed text-[var(--muted)]">
            {t(description)}
          </p>
        )}
      </div>
      {actions && (
        <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>
      )}
    </div>
  );
}
