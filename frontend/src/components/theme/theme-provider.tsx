import { useLocale, t } from "@/lib/i18n";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { flushSync } from "react-dom";

type Theme = "light" | "dark";
type ThemeContextValue = {
  theme: Theme;
  resolved: Theme;
  setTheme: (theme: Theme, origin?: { x: number; y: number }) => void;
};
const ThemeContext = createContext<ThemeContextValue | null>(null);
function applyTheme(theme: Theme) {
  const root = document.documentElement;
  root.classList.toggle("dark", theme === "dark");
  root.classList.toggle("light", theme === "light");
  root.style.colorScheme = theme;
  document
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute("content", theme === "dark" ? "#101318" : "#f5f6f8");
}
export function ThemeProvider({ children }: { children: ReactNode }) {
  useLocale();
  const [theme, setThemeState] = useState<Theme>(() =>
    localStorage.getItem("veloray-theme") === "light" ? "light" : "dark",
  );
  const transition = useRef<ViewTransition | null>(null);
  useEffect(() => applyTheme(theme), [theme]);
  const setTheme = useCallback(
    (next: Theme, origin?: { x: number; y: number }) => {
      if (next === theme) return;
      transition.current?.skipTransition();
      const update = () => {
        localStorage.setItem("veloray-theme", next);
        applyTheme(next);
        flushSync(() => setThemeState(next));
      };
      if (
        !document.startViewTransition ||
        matchMedia("(prefers-reduced-motion: reduce)").matches
      ) {
        update();
        return;
      }
      const x = Math.max(0, Math.min(innerWidth, origin?.x ?? innerWidth / 2)),
        y = Math.max(0, Math.min(innerHeight, origin?.y ?? 80));
      const radius = Math.hypot(
        Math.max(x, innerWidth - x),
        Math.max(y, innerHeight - y),
      );
      const current = document.startViewTransition(update);
      transition.current = current;
      void current.ready
        .then(() => {
          document.documentElement.animate(
            {
              clipPath: [
                `circle(0px at ${x}px ${y}px)`,
                `circle(${radius}px at ${x}px ${y}px)`,
              ],
            },
            {
              duration: 520,
              easing: "cubic-bezier(.22,1,.36,1)",
              pseudoElement: "::view-transition-new(root)",
            },
          );
        })
        .catch(() => {});
      void current.finished
        .then(() => {
          if (transition.current === current) transition.current = null;
        })
        .catch(() => {});
    },
    [theme],
  );
  const value = useMemo(
    () => ({ theme, resolved: theme, setTheme }),
    [theme, setTheme],
  );
  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  );
}
export function useTheme() {
  const value = useContext(ThemeContext);
  if (!value) throw new Error(t("useTheme must be used within ThemeProvider"));
  return value;
}
