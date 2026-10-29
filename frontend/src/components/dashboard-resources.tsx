import { useId } from "react";
import { useLocale, t } from "@/lib/i18n";
import { smoothPath } from "@/lib/chart";
import { FaIcon } from "@/components/ui/fa-icon";
import { cn } from "@/lib/cn";

export type ResourceSample = {
  at: number;
  cpu: number | null;
  memory: number | null;
  swap: number | null;
  disk: number | null;
};

export function DashboardSparkline({
  values,
  label,
  blue = false,
  className,
}: {
  values: number[];
  label: string;
  blue?: boolean;
  className?: string;
}) {
  const id = `resource-${useId().replace(/:/g, "")}`;
  const measured = values.filter(Number.isFinite);
  const max = Math.max(1, ...measured);
  const points = measured.map((value, index) => ({
    x: (index / Math.max(1, measured.length - 1)) * 300,
    y: 44 - (Math.max(0, value) / max) * 35,
  }));
  const line = smoothPath(points);
  return (
    <svg
      className={cn("dashboard-sparkline", blue && "sparkline-blue", className)}
      viewBox="0 0 300 48"
      preserveAspectRatio="none"
      role="img"
      aria-label={label}
    >
      <title>{label}</title>
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="currentColor" stopOpacity=".19" />
          <stop offset="1" stopColor="currentColor" stopOpacity=".01" />
        </linearGradient>
      </defs>
      <path d="M0 46H300" className="sparkline-baseline" />
      {points.length > 1 ? (
        <>
          <path d={`${line} L 300 48 L 0 48 Z`} fill={`url(#${id})`} />
          <path d={line} className="sparkline-trace" />
        </>
      ) : points.length === 1 ? (
        <circle cx="298" cy={points[0].y} r="2" fill="currentColor" />
      ) : null}
    </svg>
  );
}

export function ResourceCard({
  label,
  icon,
  value,
  note,
  samples,
}: {
  label: string;
  icon: string;
  value: number | undefined;
  note: string;
  samples: (number | null)[];
}) {
  useLocale();
  const valid = samples.filter(
    (sample): sample is number =>
      typeof sample === "number" && Number.isFinite(sample),
  );
  const available = typeof value === "number" && Number.isFinite(value);
  const average = valid.length
    ? valid.reduce((sum, sample) => sum + sample, 0) / valid.length
    : null;
  const peak = valid.length ? Math.max(...valid) : null;
  return (
    <article
      className={cn(
        "dashboard-resource",
        available && value > 85 && "resource-warning",
      )}
    >
      <div className="resource-heading">
        <FaIcon icon={icon} />
        <h3>{label}</h3>
      </div>
      <div className="resource-value">
        {available ? value.toFixed(1) : "—"}
        {available && <small>%</small>}
      </div>
      <p className="resource-note">{note}</p>
      <div className="resource-statistics">
        <span>
          {t("Average", "میانگین")}{" "}
          <b>{average === null ? "—" : `${average.toFixed(1)}%`}</b>
        </span>
        <span>
          {t("Peak", "بیشینه")}{" "}
          <b>{peak === null ? "—" : `${peak.toFixed(1)}%`}</b>
        </span>
      </div>
      <div className="resource-chart">
        <DashboardSparkline
          values={valid}
          label={`${label} — ${t("Samples collected while this dashboard is open", "نمونه‌های ثبت‌شده از زمان بازشدن داشبورد")}`}
        />
        {valid.length < 2 && (
          <span className="resource-collecting">
            {t("Collecting samples…", "در حال ثبت نمونه‌ها…")}
          </span>
        )}
      </div>
    </article>
  );
}
