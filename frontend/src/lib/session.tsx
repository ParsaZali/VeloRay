import { createContext, useContext } from "react";
import type { Session } from "./api";
export const SessionContext = createContext<Session | null>(null);
export const useSession = () => useContext(SessionContext);
export const isReseller = (session: Session | null) =>
  !!session && !session.is_superuser && session.role === "reseller";
const resellerPermissions = new Set([
  "dashboard.read",
  "clients.read",
  "clients.write",
  "clients.delete",
  "traffic.read",
  "settings.read",
  "commerce.read",
  "commerce.write",
  "resellers.read",
  "resellers.write",
]);
export const can = (session: Session | null, permission: string) =>
  !!session?.is_superuser ||
  (!!session?.permissions?.includes(permission) &&
    (!isReseller(session) || resellerPermissions.has(permission)));
