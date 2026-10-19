import { SubscriptionPoliciesPage } from "@/pages/subscription-policies";
import { CustomersPage, ServicesPage } from "@/pages/customers";
import { ProfilesPage } from "@/pages/profiles";
import { OutboundsPage, RoutingPage } from "@/pages/network";
import { SessionContext, can } from "@/lib/session";
import { useLocale, t } from "@/lib/i18n";
import { Suspense, useEffect, useMemo, useState } from "react";
import { Shell } from "@/components/layout/shell";
import type { RouteKey } from "@/components/layout/sidebar";
import { Dashboard as Dashboard } from "@/pages/dashboard";
import { NodesPage as NodesPage } from "@/pages/nodes";
import { InboundsPage as InboundsPage } from "@/pages/inbounds";
import { ImportsPage as ImportsPage } from "@/pages/imports";
import { ClientsPage as ClientsPage } from "@/pages/clients";
import { SecurityPage as SecurityPage } from "@/pages/security";
import { SettingsPage as SettingsPage } from "@/pages/settings";
import { XrayPage as XrayPage } from "@/pages/xray";
import { ApiDocsPage as ApiDocsPage } from "@/pages/api-docs";
import { AdminPage as AdminPage } from "@/pages/admin";
import { LoginPage } from "@/pages/login";
import {
  api,
  currentSession,
  logout,
  type AppSettings,
  type Session,
} from "@/lib/api";
import { VeloRayMark } from "@/components/brand/veloray-mark";

const meta: Record<RouteKey, [string, string]> = {
  "subscription-policies": ["Subscription policies", ""],
  customers: ["Customers", ""],
  services: ["Services and plans", ""],
  profiles: ["Config profiles", ""],
  dashboard: ["Dashboard", "Traffic and server status"],
  imports: ["Import data", "Review and migrate panel backups"],
  clients: ["Clients", "Credentials, quotas, expiry and subscription access"],
  outbounds: ["Outbounds", ""],
  routing: ["Routing", ""],
  inbounds: ["Inbounds", "Ports, protocols and connection security"],
  nodes: ["Nodes", "Local server and remote VeloRay agents"],
  xray: ["Xray Core", "Service health and configuration"],
  security: ["Security", "Administrator 2FA and account protection"],
  api: ["API", "REST endpoints and automation reference"],
  settings: ["Settings", "Panel defaults and server information"],
  accounts: ["Accounts", "Panel operators, roles and account security"],
};
const routes: Record<string, RouteKey> = {
  "/": "dashboard",
  "/dashboard": "dashboard",
  "/clients": "clients",
  "/subscription-policies": "subscription-policies",
  "/customers": "customers",
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
  dashboard: "/dashboard",
  clients: "/clients",
  "subscription-policies": "/subscription-policies",
  customers: "/customers",
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

export default function App() {
  const { setLocale } = useLocale();
  const [user, setUser] = useState<Session | null | undefined>(undefined);
  const initial = routes[window.location.pathname] || "dashboard";
  const [route, setRouteState] = useState<RouteKey>(initial);
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
        document.documentElement.dataset.accent = s.accent || "emerald";
        document.title = `${s.site_name || "VeloRay"}`;
      })
      .catch(() => {});
  }, [user]);
  useEffect(() => {
    if (user?.two_factor_setup_required) {
      setRouteState("security");
      if (window.location.pathname !== "/security")
        history.replaceState({}, "", "/security");
    }
  }, [user?.two_factor_setup_required]);
  useEffect(() => {
    const f = () =>
      setRouteState(routes[window.location.pathname] || "dashboard");
    window.addEventListener("popstate", f);
    return () => window.removeEventListener("popstate", f);
  }, []);
  useEffect(() => {
    if (!user || user.two_factor_setup_required) return;
    if (
      (route === "accounts" ||
        route === "imports" ||
        route === "profiles" ||
        route === "subscription-policies") &&
      !user.is_superuser
    ) {
      setRouteState("dashboard");
      history.replaceState({}, "", "/dashboard");
    } else if (
      ["/admin", "/audit", "/subscriptions", "/traffic"].includes(
        window.location.pathname,
      )
    )
      history.replaceState({}, "", paths[route]);
  }, [user, route]);
  const setRoute = (r: RouteKey) => {
    setRouteState(r);
    const p = paths[r];
    if (window.location.pathname !== p) history.pushState({}, "", p);
  };
  const page = useMemo(() => {
    const access: Partial<Record<RouteKey, string>> = {
      dashboard: "dashboard.read",
      clients: "clients.read",
      customers: "clients.read",
      services: "clients.read",
      inbounds: "inbounds.read",
      outbounds: "outbounds.read",
      routing: "routing_rules.read",
      nodes: "nodes.read",
      xray: "xray.read",
      api: "api.read",
    };
    if (user && access[route] && !can(user, access[route]!))
      return (
        <div role="status">{t("Access is not assigned to this account.")}</div>
      );
    switch (route) {
      case "subscription-policies":
        return user?.is_superuser ? (
          <SubscriptionPoliciesPage />
        ) : (
          <SettingsPage />
        );
      case "customers":
        return <CustomersPage />;
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
  }, [route, user?.is_superuser]);
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
  const [title, subtitle] = meta[route];
  return (
    <SessionContext.Provider value={user}>
      <Shell
        route={route}
        setRoute={setRoute}
        title={t(title)}
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
