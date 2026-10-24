import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { usePanelUpdates } from "@/lib/panel-updates";
import { isReseller, useSession } from "@/lib/session";
import { canVisit, type RouteKey } from "@/lib/navigation";
export type { RouteKey } from "@/lib/navigation";
import { useLocale, t } from "@/lib/i18n";
import { ChevronLeft, ExternalLink, X } from "lucide-react";
import { cn } from "@/lib/cn";
import { VeloRayWordmark } from "@/components/brand/veloray-mark";
import { FaIcon } from "@/components/ui/fa-icon";
const groups = [
  {
    label: "Overview",
    items: [{ key: "dashboard", label: "Dashboard", icon: "gauge-high" }],
  },
  {
    label: "Management",
    items: [
      { key: "clients", label: "Clients", icon: "users" },
      { key: "inbounds", label: "Inbounds", icon: "diagram-project" },
      {
        key: "outbounds",
        label: "Outbounds",
        icon: "arrow-right-from-bracket",
      },
      { key: "routing", label: "Routing", icon: "route" },
    ],
  },
  {
    label: "Infrastructure",
    items: [
      { key: "nodes", label: "Nodes", icon: "server" },
      { key: "operations", label: "Operations", icon: "gauge-high" },
      { key: "xray", label: "Cores", icon: "microchip" },
    ],
  },
  {
    label: "System",
    items: [
      { key: "security", label: "Security", icon: "shield-halved" },
      { key: "api", label: "API", icon: "code" },
      { key: "accounts", label: "Accounts", icon: "user-shield" },
      { key: "settings", label: "Settings", icon: "gear" },
    ],
  },
  {
    label: "Commerce",
    items: [
      { key: "services", label: "Services and plans", icon: "layer-group" },
    ],
  },
] as const;
export function Sidebar({
  route,
  setRoute,
  collapsed,
  setCollapsed,
  mobileOpen,
  setMobileOpen,
}: {
  route: RouteKey;
  setRoute: (r: RouteKey) => void;
  collapsed: boolean;
  setCollapsed: (v: boolean) => void;
  mobileOpen: boolean;
  setMobileOpen: (v: boolean) => void;
}) {
  useLocale();
  const session = useSession();
  const { status } = usePanelUpdates(Boolean(session?.is_superuser));
  const [version, setVersion] = useState("");
  useEffect(() => {
    const c = new AbortController();
    void api<{ version: string }>("/version", { signal: c.signal })
      .then((data) => {
        if (!c.signal.aborted) setVersion(data.version);
      })
      .catch(() => {});
    return () => c.abort();
  }, []);
  const visibleGroups = groups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => canVisit(session, item.key)),
    }))
    .filter((group) => group.items.length > 0);
  const itemLabel = (key: RouteKey, label: string) =>
    key === "services" && isReseller(session)
      ? t("Sales & wallet", "فروش و کیف پول")
      : t(label);
  const body = (compact: boolean) => (
    <>
      <div className="flex min-h-20 shrink-0 items-center justify-between px-4 py-3">
        <div className="min-w-0">
          <VeloRayWordmark compact={compact} />
          {!compact && (
            <div
              className="mt-1.5 flex flex-wrap items-center gap-1.5 text-[10px]"
              dir="ltr"
            >
              <span className="text-[var(--muted)]">
                {version ? `v${version}` : "…"}
              </span>
              {status?.state === "up_to_date" ? (
                <span className="text-emerald-600 dark:text-emerald-400">
                  (Up to date)
                </span>
              ) : status?.state === "available" ? (
                <button
                  type="button"
                  className="text-amber-600 dark:text-amber-400 hover:underline"
                  onClick={() => {
                    setRoute("updates");
                    setMobileOpen(false);
                  }}
                >
                  (Update available)
                </button>
              ) : status?.state === "verifying" ? (
                <span className="text-amber-600 dark:text-amber-400">
                  (Verification pending)
                </span>
              ) : null}
            </div>
          )}
        </div>
        <button
          aria-label={t("Close menu")}
          className="grid h-8 w-8 place-items-center rounded-md text-[var(--muted)] hover:bg-[var(--surface-2)] lg:hidden"
          onClick={() => setMobileOpen(false)}
        >
          <X className="h-4 w-4" />
        </button>
      </div>
      <div className="mx-3 h-px bg-[var(--border-subtle)]" />
      <nav className="scrollbar-none flex-1 overflow-y-auto px-3 py-4">
        {visibleGroups.map((g) => (
          <div key={g.label} className="mb-5">
            {!compact && (
              <div className="mb-1.5 px-2 text-[10px] font-medium text-[var(--muted)]">
                {t(g.label)}
              </div>
            )}
            <div className="space-y-1">
              {g.items.map((item) => {
                const active =
                  route === item.key ||
                  (item.key === "settings" &&
                    [
                      "profiles",
                      "subscription-policies",
                      "imports",
                      "updates",
                    ].includes(route));
                return (
                  <button
                    key={item.key}
                    aria-label={itemLabel(item.key, item.label)}
                    aria-current={active ? "page" : undefined}
                    title={
                      compact ? itemLabel(item.key, item.label) : undefined
                    }
                    onClick={() => {
                      setRoute(item.key);
                      setMobileOpen(false);
                    }}
                    className={cn(
                      "group flex h-9 w-full min-w-0 items-center rounded-md text-[13px] font-medium transition",
                      compact ? "justify-center px-0" : "gap-2.5 px-2.5",
                      active
                        ? "sidebar-current bg-[var(--surface-2)] text-[var(--fg)]"
                        : "text-[var(--muted-strong)] hover:bg-[var(--surface-2)] hover:text-[var(--fg)]",
                    )}
                  >
                    <FaIcon
                      icon={item.icon}
                      className={cn(
                        "text-[12px] transition-colors",
                        active
                          ? "text-[var(--fg)]"
                          : "text-[var(--muted-strong)] group-hover:text-[var(--fg)]",
                      )}
                    />
                    {!compact && (
                      <span className="truncate">
                        {itemLabel(item.key, item.label)}
                      </span>
                    )}
                  </button>
                );
              })}
            </div>
          </div>
        ))}
      </nav>
      <div className="sidebar-footer border-t border-[var(--border-subtle)] p-3">
        {!compact && (
          <a
            href="https://github.com/ParsaZali/VeloRay"
            target="_blank"
            rel="noopener noreferrer"
            aria-label="VeloRay GitHub"
            className="mb-2 flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-[var(--muted-strong)] transition hover:bg-[var(--surface-2)] hover:text-[var(--fg)]"
          >
            <VeloRayWordmark />
            <ExternalLink className="ml-auto h-3.5 w-3.5 shrink-0 text-[var(--muted)]" />
          </a>
        )}
        <button
          aria-label={
            compact ? t("Expand navigation") : t("Collapse navigation")
          }
          onClick={() => setCollapsed(!collapsed)}
          className="hidden h-8 w-full items-center justify-center rounded-md text-[var(--muted)] hover:bg-[var(--surface-2)] lg:flex"
        >
          <ChevronLeft
            className={cn(
              "h-4 w-4 transition-transform",
              compact && "rotate-180",
            )}
          />
        </button>
      </div>
    </>
  );
  return (
    <>
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-40 hidden border-r border-[var(--border)] bg-[var(--sidebar)] lg:flex lg:flex-col transition-[width] duration-200",
          collapsed ? "w-[68px]" : "w-[252px]",
        )}
      >
        {body(collapsed)}
      </aside>
      {mobileOpen && (
        <div className="fixed inset-0 z-50 lg:hidden">
          <button
            aria-label={t("Close menu")}
            onClick={() => setMobileOpen(false)}
            className="absolute inset-0 bg-black/55 backdrop-blur-[3px]"
          />
          <aside className="absolute inset-y-0 left-0 flex w-[286px] flex-col border-r border-[var(--border)] bg-[var(--sidebar)] shadow-2xl">
            {body(false)}
          </aside>
        </div>
      )}
    </>
  );
}
