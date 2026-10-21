import { useLocale } from "@/lib/i18n";
import type { HTMLAttributes, PropsWithChildren } from "react";
import { cn } from "@/lib/cn";
export function Card({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  useLocale();
  return (
    <div
      className={cn(
        "min-w-0 rounded-lg border border-[var(--border)] bg-[var(--card)] shadow-[var(--card-shadow)]",
        className,
      )}
      {...props}
    />
  );
}
export function CardHeader({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  useLocale();
  return (
    <div
      className={cn(
        "flex min-w-0 flex-wrap items-start justify-between gap-4 border-b border-[var(--border-subtle)] px-5 py-4",
        className,
      )}
      {...props}
    />
  );
}
export function CardContent({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  useLocale();
  return <div className={cn("min-w-0 p-5", className)} {...props} />;
}
export function CardTitle({
  className,
  children,
  ...props
}: PropsWithChildren<HTMLAttributes<HTMLHeadingElement>>) {
  useLocale();
  return (
    <h3
      className={cn(
        "min-w-0 text-[15px] font-semibold tracking-[-.01em] text-[var(--fg)]",
        className,
      )}
      {...props}
    >
      {children}
    </h3>
  );
}
