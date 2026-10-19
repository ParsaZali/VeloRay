import { localeName } from "./i18n";
export function datePreferences() {
  const calendar = localStorage.getItem("veloray-calendar") || "auto";
  const timezone =
    localStorage.getItem("veloray-timezone") ||
    Intl.DateTimeFormat().resolvedOptions().timeZone;
  return { calendar, timezone };
}
export function formatDate(
  value: string | number | Date | null | undefined,
  options: Intl.DateTimeFormatOptions = {},
) {
  if (value == null || value === "") return "—";
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "—";
  const { calendar, timezone } = datePreferences();
  const settings: Intl.DateTimeFormatOptions = {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    ...options,
    timeZone: timezone,
    ...(calendar === "auto" ? {} : { calendar }),
  };
  try {
    return new Intl.DateTimeFormat(localeName(), settings).format(date);
  } catch {
    return new Intl.DateTimeFormat(localeName(), {
      ...settings,
      timeZone: "UTC",
      calendar: localeName() === "fa-IR" ? "persian" : "gregory",
    }).format(date);
  }
}
export function saveDatePreferences(calendar: string, timezone: string) {
  if (!["auto", "persian", "gregory"].includes(calendar))
    throw new Error("Invalid calendar");
  new Intl.DateTimeFormat("en", { timeZone: timezone }).format(new Date());
  localStorage.setItem("veloray-calendar", calendar);
  localStorage.setItem("veloray-timezone", timezone);
  window.dispatchEvent(new Event("veloray-date-settings"));
}
