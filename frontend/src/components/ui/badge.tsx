import { useLocale, t } from "@/lib/i18n";
import type { HTMLAttributes } from "react";
import { cn } from "@/lib/cn";
type Props = HTMLAttributes<HTMLSpanElement> & {
  tone?: "green" | "amber" | "red" | "neutral" | "blue";
};
export function Badge({ className, tone = "neutral", ...props }: Props) {
  useLocale();
  const tones = {
    green: "border-emerald-500/20 bg-emerald-500/10 text-emerald-500",
    amber: "border-amber-500/20 bg-amber-500/10 text-amber-500",
    red: "border-red-500/20 bg-red-500/10 text-red-500",
    neutral:
      "border-[var(--border)] bg-[var(--surface-2)] text-[var(--muted-strong)]",
    blue: "border-sky-500/20 bg-sky-500/10 text-sky-500",
  };
  return (
    <span
      className={cn(
        `status-badge badge-${tone} inline-flex max-w-full items-center gap-1 rounded-md border px-2 py-1 text-[10px] font-semibold leading-none`,
        tones[tone],
        className,
      )}
      {...props}
    >
      {typeof props.children === "string" ? t(props.children) : props.children}
    </span>
  );
}
