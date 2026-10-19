import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { fa } from "./fa";
export type Locale = "en" | "fa";
let activeLocale: Locale =
  localStorage.getItem("veloray-locale") === "fa" ||
  (!localStorage.getItem("veloray-locale") &&
    localStorage.getItem("veloray-login-locale") === "fa")
    ? "fa"
    : "en";
export function t(source: string, persian?: string): string {
  if (activeLocale === "en") return source;
  return persian ?? fa[source] ?? source;
}
export const localeName = () => (activeLocale === "fa" ? "fa-IR" : "en-US");
const LocaleContext = createContext({
  locale: activeLocale,
  setLocale: (_locale: Locale) => {},
});
export const useLocale = () => useContext(LocaleContext);
export function LocaleProvider({ children }: { children: ReactNode }) {
  const [locale, update] = useState<Locale>(activeLocale);
  const [, refreshDates] = useState(0);
  useEffect(() => {
    const f = () => refreshDates((n) => n + 1);
    window.addEventListener("veloray-date-settings", f);
    return () => window.removeEventListener("veloray-date-settings", f);
  }, []);
  activeLocale = locale;
  document.documentElement.lang = locale;
  document.documentElement.dir = locale === "fa" ? "rtl" : "ltr";
  const setLocale = (next: Locale) => {
    localStorage.setItem("veloray-locale", next);
    localStorage.setItem("veloray-login-locale", next);
    activeLocale = next;
    update(next);
  };
  return (
    <LocaleContext.Provider value={{ locale, setLocale }}>
      {children}
    </LocaleContext.Provider>
  );
}
export function tx(source: string, ...values: unknown[]): string {
  return t(source).replace(/\{(\d+)\}/g, (_, index: string) =>
    String(values[Number(index)] ?? ""),
  );
}
