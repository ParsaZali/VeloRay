import { can, useSession } from "@/lib/session";
import { useLocale, t } from "@/lib/i18n";
import { ChevronLeft, ExternalLink, X } from "lucide-react";
import { cn } from "@/lib/cn";
import { VeloRayWordmark } from "@/components/brand/veloray-mark";
import { FaIcon } from "@/components/ui/fa-icon";
export type RouteKey =
  | "dashboard"
  | "customers"
  | "services"
  | "subscription-policies"
  | "profiles"
  | "clients"
  | "outbounds"
  | "routing"
  | "inbounds"
  | "nodes"
  | "xray"
  | "security"
  | "settings"
  | "api"
  | "accounts"
  | "imports";
const groups = [
  {
    label: "Overview",
    items: [{ key: "dashboard", label: "Dashboard", icon: "gauge-high" }],
  },
  {
    label: "Management",
    items: [
      { key: "customers", label: "Customers", icon: "users" },
      { key: "services", label: "Services and plans", icon: "layer-group" },
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
      { key: "profiles", label: "Config profiles", icon: "file-code" },
      {
        key: "subscription-policies",
        label: "Subscription policies",
        icon: "link",
      },
      { key: "xray", label: "Xray Core", icon: "microchip" },
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
] as const;
export function Sidebar({
  route,
  setRoute,
  collapsed,
  setCollapsed,
  mobileOpen,
  setMobileOpen,
  isSuperuser = false,
}: {
  route: RouteKey;
  setRoute: (r: RouteKey) => void;
  collapsed: boolean;
  setCollapsed: (v: boolean) => void;
  mobileOpen: boolean;
  setMobileOpen: (v: boolean) => void;
  isSuperuser?: boolean;
}) {
  useLocale();
  const session = useSession();
  const access: Record<string, string> = {
    dashboard: "dashboard.read",
    clients: "clients.read",
    customers: "clients.read",
    services: "clients.read",
    inbounds: "inbounds.read",
    nodes: "nodes.read",
    outbounds: "outbounds.read",
    routing: "routing_rules.read",
    xray: "xray.read",
    api: "api.read",
  };
  const body = (compact: boolean) => (
    <>
      <div className="flex h-16 items-center justify-between px-4">
        <VeloRayWordmark compact={compact} />
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
        {groups.map((g) => (
          <div key={g.label} className="mb-5">
            {!compact && (
              <div className="mb-1.5 px-2 text-[9px] font-medium text-[var(--muted)]">
                {t(g.label)}
              </div>
            )}
            <div className="space-y-1">
              {g.items
                .filter(
                  (item) =>
                    (![
                      "accounts",
                      "imports",
                      "profiles",
                      "subscription-policies",
                    ].includes(item.key) ||
                      isSuperuser) &&
                    (!access[item.key] || can(session, access[item.key])),
                )
                .map((item) => {
                  const active = route === item.key;
                  return (
                    <button
                      key={item.key}
                      aria-label={t(item.label)}
                      aria-current={active ? "page" : undefined}
                      title={compact ? t(item.label) : undefined}
                      onClick={() => {
                        setRoute(item.key);
                        setMobileOpen(false);
                      }}
                      className={cn(
                        "group flex h-9 w-full min-w-0 items-center rounded-md text-[12px] font-medium transition",
                        compact ? "justify-center px-0" : "gap-2.5 px-2.5",
                        active
                          ? "bg-[var(--surface-2)] text-[var(--fg)]"
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
                        <span className="truncate">{t(item.label)}</span>
                      )}
                    </button>
                  );
                })}
            </div>
          </div>
        ))}
      </nav>
      <div className="border-t border-[var(--border-subtle)] p-3">
        {!compact && (
          <a
            href="https://github.com/ParsaZali/VeloRay"
            target="_blank"
            rel="noopener noreferrer"
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
