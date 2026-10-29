import { useLocale } from "@/lib/i18n";
import type { ButtonHTMLAttributes } from "react";
import { cn } from "@/lib/cn";
type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "default" | "secondary" | "ghost" | "outline" | "danger";
  size?: "sm" | "md" | "icon";
};
export function Button({
  className,
  variant = "default",
  size = "md",
  type = "button",
  ...props
}: Props) {
  useLocale();
  const variants = {
    default:
      "border border-[var(--primary)] bg-[var(--primary)] text-[var(--primary-fg)] shadow-sm hover:opacity-90",
    secondary:
      "border border-[var(--border)] bg-[var(--surface-2)] text-[var(--fg)] hover:bg-[var(--surface-3)]",
    ghost:
      "border border-transparent text-[var(--muted-strong)] hover:bg-[var(--surface-2)] hover:text-[var(--fg)]",
    outline:
      "border border-[var(--border)] bg-[var(--card)] text-[var(--fg)] shadow-sm hover:bg-[var(--surface-2)]",
    danger:
      "border border-red-500/20 bg-red-500/10 text-red-500 hover:bg-red-500/15",
  };
  const sizes = {
    sm: "h-8 rounded-md px-2.5 text-[11px]",
    md: "h-9 rounded-md px-3.5 text-[12px]",
    icon: "h-9 w-9 rounded-md p-0",
  };
  return (
    <button
      aria-label={props["aria-label"] || props.title}
      type={type}
      data-size={size}
      className={cn(
        "panel-button inline-flex min-w-0 max-w-full items-center justify-center gap-2 overflow-hidden whitespace-nowrap font-medium outline-none transition focus-visible:ring-2 focus-visible:ring-[var(--ring)] disabled:pointer-events-none disabled:opacity-45",
        variants[variant],
        sizes[size],
        className,
      )}
      {...props}
    />
  );
}
