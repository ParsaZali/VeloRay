import { PWAControls } from "@/components/pwa-controls";
import { tx } from "@/lib/i18n";
import { t, useLocale } from "@/lib/i18n";
import { LogOut, Menu, Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTheme } from "@/components/theme/theme-provider";
import type { Session } from "@/lib/api";
import { VeloRayMark } from "@/components/brand/veloray-mark";

export function Header({
  title,
  subtitle,
  user,
  onMenu,
  onLogout,
}: {
  title: string;
  subtitle: string;
  user: Session;
  onMenu: () => void;
  onLogout: () => void;
}) {
  const { locale, setLocale } = useLocale();
  const { theme, setTheme } = useTheme();
  const next = theme === "dark" ? "light" : "dark";
  const ThemeIcon = theme === "dark" ? Moon : Sun;
  return (
    <header className="panel-header sticky top-0 z-30 flex h-16 items-center justify-between gap-3 border-b border-[var(--border)] px-4 sm:px-6">
      <div className="flex min-w-0 items-center gap-3">
        <Button
          variant="ghost"
          size="icon"
          className="lg:hidden"
          aria-label={t("Open navigation")}
          onClick={onMenu}
        >
          <Menu className="h-4 w-4" />
        </Button>
        <div className="grid h-8 w-8 shrink-0 place-items-center lg:hidden">
          <VeloRayMark className="h-8 w-8" />
        </div>
        <div className="min-w-0">
          <div className="truncate text-[15px] font-semibold tracking-[-.01em] text-[var(--fg)]">
            {title}
          </div>
          <div className="hidden truncate text-[12px] text-[var(--muted)] sm:block">
            {subtitle}
          </div>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        <PWAControls />
        <Button
          variant="ghost"
          size="sm"
          aria-label={t("Change language")}
          onClick={() => setLocale(locale === "en" ? "fa" : "en")}
        >
          {locale === "en" ? t("Persian") : "انگلیسی"}
        </Button>
        <Button
          title={tx("Theme: {0}", t(theme))}
          variant="ghost"
          size="icon"
          onClick={(event) => {
            const rect = event.currentTarget.getBoundingClientRect();
            setTheme(next, {
              x: rect.left + rect.width / 2,
              y: rect.top + rect.height / 2,
            });
          }}
        >
          <ThemeIcon className="h-4 w-4" />
        </Button>
        <div className="mx-1 hidden h-5 w-px bg-[var(--border)] sm:block" />
        <div className="hidden min-w-0 items-center gap-2 sm:flex">
          <div className="grid h-8 w-8 shrink-0 place-items-center rounded-full border border-[var(--border)] bg-[var(--surface-2)] text-[10px] font-semibold uppercase text-[var(--fg)]">
            {user.username.slice(0, 2)}
          </div>
          <div className="max-w-28 truncate text-[13px] font-medium text-[var(--muted-strong)]">
            {user.username}
          </div>
        </div>
        <Button
          title={t("Sign out")}
          variant="ghost"
          size="icon"
          onClick={onLogout}
        >
          <LogOut className="h-4 w-4" />
        </Button>
      </div>
    </header>
  );
}
