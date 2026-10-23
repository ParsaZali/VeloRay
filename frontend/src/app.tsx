import { SessionContext, isReseller } from "@/lib/session";
import { canVisit } from "@/lib/navigation";
import { useLocale, t } from "@/lib/i18n";
import { lazy, Suspense, useEffect, useMemo, useState } from "react";
import { Shell } from "@/components/layout/shell";
import type { RouteKey } from "@/components/layout/sidebar";
import { LoginPage } from "@/pages/login";
import {
  api,
  currentSession,
  logout,
  type AppSettings,
  type Session,
} from "@/lib/api";
import { VeloRayMark } from "@/components/brand/veloray-mark";

const OperationsPage = lazy(() =>
  import("@/pages/operations").then((module) => ({
    default: module.OperationsPage,
  })),
);
const SubscriptionPoliciesPage = lazy(() =>
  import("@/pages/subscription-policies").then((module) => ({
    default: module.SubscriptionPoliciesPage,
  })),
);
const ServicesPage = lazy(() =>
  import("@/pages/customers").then((module) => ({
    default: module.ServicesPage,
  })),
);
const ProfilesPage = lazy(() =>
  import("@/pages/profiles").then((module) => ({
    default: module.ProfilesPage,
  })),
);
const OutboundsPage = lazy(() =>
  import("@/pages/network").then((module) => ({
    default: module.OutboundsPage,
  })),
);
const RoutingPage = lazy(() =>
  import("@/pages/network").then((module) => ({ default: module.RoutingPage })),
);
const Dashboard = lazy(() =>
  import("@/pages/dashboard").then((module) => ({ default: module.Dashboard })),
);
const NodesPage = lazy(() =>
  import("@/pages/nodes").then((module) => ({ default: module.NodesPage })),
);
const InboundsPage = lazy(() =>
  import("@/pages/inbounds").then((module) => ({
    default: module.InboundsPage,
  })),
);
const ImportsPage = lazy(() =>
  import("@/pages/imports").then((module) => ({ default: module.ImportsPage })),
);
const ClientsPage = lazy(() =>
  import("@/pages/clients").then((module) => ({ default: module.ClientsPage })),
);
const SecurityPage = lazy(() =>
  import("@/pages/security").then((module) => ({
    default: module.SecurityPage,
  })),
);
const SettingsPage = lazy(() =>
  import("@/pages/settings").then((module) => ({
    default: module.SettingsPage,
  })),
);
const XrayPage = lazy(() =>
  import("@/pages/xray").then((module) => ({ default: module.XrayPage })),
);
const ApiDocsPage = lazy(() =>
  import("@/pages/api-docs").then((module) => ({
    default: module.ApiDocsPage,
  })),
);
const AdminPage = lazy(() =>
  import("@/pages/admin").then((module) => ({ default: module.AdminPage })),
);

const meta: Record<RouteKey, [string, string]> = {
  operations: ["Operations", ""],
  "subscription-policies": ["Subscription policies", ""],
  services: ["Services and plans", ""],
  profiles: ["Config profiles", ""],
  dashboard: ["Dashboard", "Traffic and server status"],
  imports: ["Import data", "Review and migrate panel backups"],
  clients: ["Clients", "Credentials, quotas, expiry and subscription access"],
  outbounds: ["Outbounds", ""],
  routing: ["Routing", ""],
  inbounds: ["Inbounds", "Ports, protocols and connection security"],
  nodes: ["Nodes", "Local server and remote VeloRay agents"],
  xray: ["Cores", "Service health and configuration"],
  security: ["Security", "Administrator 2FA and account protection"],
  api: ["API", "REST endpoints and automation reference"],
  settings: ["Settings", "Panel defaults and server information"],
  accounts: ["Accounts", "Panel operators, roles and account security"],
};
const routes: Record<string, RouteKey> = {
  "/operations": "operations",
  "/": "dashboard",
  "/dashboard": "dashboard",
  "/clients": "clients",
  "/subscription-policies": "subscription-policies",
  "/customers": "clients",
  "/services": "services",
  "/profiles": "profiles",
  "/imports": "imports",
  "/inbounds": "inbounds",
  "/outbounds": "outbounds",
  "/routing": "routing",
  "/nodes": "nodes",
  "/subscriptions": "clients",
  "/traffic": "dashboard",
  "/xray": "xray",
  "/audit": "dashboard",
  "/security": "security",
  "/api-docs": "api",
  "/settings": "settings",
  "/admin": "accounts",
  "/accounts": "accounts",
};
const paths: Record<RouteKey, string> = {
  operations: "/operations",
  dashboard: "/dashboard",
  clients: "/clients",
  "subscription-policies": "/subscription-policies",
  services: "/services",
  profiles: "/profiles",
  imports: "/imports",
  inbounds: "/inbounds",
  outbounds: "/outbounds",
  routing: "/routing",
  nodes: "/nodes",
  xray: "/xray",
  security: "/security",
  api: "/api-docs",
  settings: "/settings",
  accounts: "/accounts",
};

const StorefrontPage = lazy(() =>
  import("@/pages/storefront").then((module) => ({
    default: module.StorefrontPage,
  })),
);
export default function App() {
  if (window.location.pathname === "/shop")
    return (
      <Suspense
        fallback={
          <div role="status" className="p-8">
            {t("Loading…")}
          </div>
        }
      >
        <StorefrontPage />
      </Suspense>
    );
  return <PanelApp />;
}
function PanelApp() {
  const { setLocale } = useLocale();
  const [user, setUser] = useState<Session | null | undefined>(undefined);
  const initial = routes[window.location.pathname] || "dashboard";
  const [route, setRouteState] = useState<RouteKey>(initial);
  const effectiveRoute: RouteKey = user?.two_factor_setup_required
    ? "security"
    : user && !canVisit(user, route)
      ? canVisit(user, "dashboard")
        ? "dashboard"
        : "security"
      : route;
  useEffect(() => {
    let active = true;
    const refresh = () =>
      currentSession()
        .then((u) => {
          if (active) setUser(u);
        })
        .catch(() => {
          if (active) setUser(null);
        });
    const expired = () => setUser(null);
    void refresh();
    window.addEventListener("veloray:session-expired", expired);
    window.addEventListener("veloray:session-update", refresh);
    return () => {
      active = false;
      window.removeEventListener("veloray:session-expired", expired);
      window.removeEventListener("veloray:session-update", refresh);
    };
  }, []);
  useEffect(() => {
    if (!user) return;
    api<AppSettings>("/settings")
      .then((s) => {
        if (!localStorage.getItem("veloray-locale"))
          setLocale(s.default_locale === "fa" ? "fa" : "en");
        document.title = `${s.site_name || "VeloRay"}`;
      })
      .catch(() => {});
  }, [user]);
  useEffect(() => {
    const f = () =>
      setRouteState(routes[window.location.pathname] || "dashboard");
    window.addEventListener("popstate", f);
    return () => window.removeEventListener("popstate", f);
  }, []);
  useEffect(() => {
    if (!user) return;
    if (route !== effectiveRoute) setRouteState(effectiveRoute);
    if (window.location.pathname !== paths[effectiveRoute])
      history.replaceState({}, "", paths[effectiveRoute]);
  }, [user, route, effectiveRoute]);
  const setRoute = (r: RouteKey) => {
    if (
      !user ||
      !canVisit(user, r) ||
      (user.two_factor_setup_required && r !== "security")
    )
      return;
    setRouteState(r);
    const p = paths[r];
    if (window.location.pathname !== p) history.pushState({}, "", p);
  };
  const page = useMemo(() => {
    if (user && !canVisit(user, effectiveRoute))
      return (
        <div role="status">{t("Access is not assigned to this account.")}</div>
      );
    switch (effectiveRoute) {
      case "operations":
        return user?.is_superuser ? <OperationsPage /> : <SettingsPage />;
      case "subscription-policies":
        return user?.is_superuser ? (
          <SubscriptionPoliciesPage />
        ) : (
          <SettingsPage />
        );
      case "services":
        return <ServicesPage />;
      case "profiles":
        return user?.is_superuser ? <ProfilesPage /> : <SettingsPage />;
      case "dashboard":
        return <Dashboard />;
      case "outbounds":
        return <OutboundsPage />;
      case "routing":
        return <RoutingPage />;
      case "nodes":
        return <NodesPage />;
      case "inbounds":
        return <InboundsPage />;
      case "imports":
        return <SettingsPage initialTab="imports" />;
      case "clients":
        return <ClientsPage />;
      case "xray":
        return <XrayPage />;
      case "security":
        return <SecurityPage />;
      case "api":
        return <ApiDocsPage />;
      case "accounts":
        return user?.is_superuser ? <AdminPage /> : <SettingsPage />;
      default:
        return <SettingsPage />;
    }
  }, [effectiveRoute, user]);
  if (user === undefined)
    return (
      <div className="grid min-h-screen place-items-center bg-[var(--bg)]">
        <div className="flex items-center gap-3 text-[12px] text-[var(--muted)]">
          <VeloRayMark className="h-8 w-8 animate-pulse" />
          {t("Loading…")}
        </div>
      </div>
    );
  if (!user)
    return <LoginPage onSuccess={() => currentSession().then(setUser)} />;
  const [title] = meta[effectiveRoute];
  return (
    <SessionContext.Provider value={user}>
      <Shell
        key={`${user.id}:${user.role}:${user.own_clients_only}:${user.permissions?.join("|")}`}
        route={effectiveRoute}
        setRoute={setRoute}
        title={
          effectiveRoute === "services" && isReseller(user)
            ? t("Sales & wallet", "فروش و کیف پول")
            : t(title)
        }
        subtitle=""
        user={user}
        onLogout={async () => {
          try {
            await logout();
          } finally {
            setUser(null);
          }
        }}
      >
        <Suspense
          fallback={
            <div
              role="status"
              className="py-12 text-center text-[var(--muted)]"
            >
              {t("Loading…")}
            </div>
          }
        >
          {page}
        </Suspense>
      </Shell>
    </SessionContext.Provider>
  );
}
