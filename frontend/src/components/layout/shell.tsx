import { useLocale, t } from "@/lib/i18n";
import type { ReactNode, CSSProperties } from "react";
import { useState } from "react";
import { Header } from "./header";
import { Sidebar, type RouteKey } from "./sidebar";
import type { Session } from "@/lib/api";
export function Shell({
  route,
  setRoute,
  title,
  subtitle,
  children,
  user,
  onLogout,
}: {
  route: RouteKey;
  setRoute: (r: RouteKey) => void;
  title: string;
  subtitle: string;
  children: ReactNode;
  user: Session;
  onLogout: () => void;
}) {
  useLocale();
  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem("veloray-sidebar") === "collapsed",
  );
  const [mobileOpen, setMobileOpen] = useState(false);
  const change = (v: boolean) => {
    setCollapsed(v);
    localStorage.setItem("veloray-sidebar", v ? "collapsed" : "expanded");
  };
  return (
    <div className="veloray-shell min-h-screen text-[var(--fg)]">
      <a href="#main-content" className="skip-link">
        {t("Skip to content")}
      </a>
      <Sidebar
        route={route}
        setRoute={setRoute}
        collapsed={collapsed}
        setCollapsed={change}
        mobileOpen={mobileOpen}
        setMobileOpen={setMobileOpen}
        isSuperuser={!!user.is_superuser}
      />
      <div
        className="shell-content min-h-screen"
        style={{ "--nav-width": collapsed ? "68px" : "252px" } as CSSProperties}
      >
        <Header
          title={title}
          subtitle={subtitle}
          user={user}
          onMenu={() => setMobileOpen(true)}
          onLogout={onLogout}
        />
        <main
          id="main-content"
          tabIndex={-1}
          className="veloray-grid min-h-[calc(100vh-64px)] px-4 py-5 sm:px-6 sm:py-6 xl:px-8"
        >
          <div className="mx-auto w-full max-w-[1500px]">{children}</div>
        </main>
      </div>
    </div>
  );
}
