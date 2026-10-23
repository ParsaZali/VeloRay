import type { Session } from "./api";
import { can, isReseller } from "./session";

export type RouteKey =
  | "operations"
  | "dashboard"
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

const administratorRoutes = new Set<RouteKey>([
  "accounts",
  "imports",
  "profiles",
  "subscription-policies",
  "operations",
]);
const resellerRoutes = new Set<RouteKey>([
  "dashboard",
  "clients",
  "services",
  "security",
]);
const access: Partial<Record<RouteKey, string>> = {
  dashboard: "dashboard.read",
  clients: "clients.read",
  services: "clients.read",
  inbounds: "inbounds.read",
  nodes: "nodes.read",
  outbounds: "outbounds.read",
  routing: "routing_rules.read",
  xray: "xray.read",
  api: "api.read",
  settings: "settings.read",
};

// One policy controls links and rendering, including old URLs and stale sessions.
export function canVisit(session: Session | null, route: RouteKey): boolean {
  if (!session) return false;
  if (isReseller(session) && !resellerRoutes.has(route)) return false;
  if (administratorRoutes.has(route) && !session.is_superuser) return false;
  const permission = access[route];
  return !permission || can(session, permission);
}
