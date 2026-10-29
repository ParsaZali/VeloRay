import {
  useLayoutEffect,
  useRef,
  useState,
  useId,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { MoreHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";

export function FloatingMenu({
  label,
  open,
  onToggle,
  onClose,
  children,
}: {
  label: string;
  open: boolean;
  onToggle: () => void;
  onClose: () => void;
  children: ReactNode;
}) {
  const id = useId();
  const anchor = useRef<HTMLDivElement>(null),
    menu = useRef<HTMLDivElement>(null),
    close = useRef(onClose);
  close.current = onClose;
  const [position, setPosition] = useState<{
    left: number;
    top: number;
  } | null>(null);
  useLayoutEffect(() => {
    if (!open) return;
    const trigger = anchor.current?.querySelector("button");
    const update = () => {
      if (!anchor.current || !menu.current) return;
      const rect = anchor.current.getBoundingClientRect(),
        box = menu.current.getBoundingClientRect();
      const left =
        document.documentElement.dir === "rtl"
          ? rect.left
          : rect.right - box.width;
      setPosition({
        left: Math.max(8, Math.min(left, innerWidth - box.width - 8)),
        top: Math.max(
          8,
          rect.bottom + box.height + 6 > innerHeight - 8
            ? rect.top - box.height - 6
            : rect.bottom + 6,
        ),
      });
    };
    update();
    // The portaled menu is initially hidden until its position is measured.
    // Move keyboard focus after React has committed the visible position.
    const focusFrame = requestAnimationFrame(() => {
      menu.current
        ?.querySelector<HTMLButtonElement>("button:not(:disabled)")
        ?.focus();
    });
    const outside = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!menu.current?.contains(target) && !anchor.current?.contains(target))
        close.current();
    };
    const key = (event: KeyboardEvent) => {
      if (event.defaultPrevented) return;
      if (event.key === "Escape") {
        event.preventDefault();
        close.current();
        trigger?.focus();
      }
      if (event.key === "Tab") {
        event.preventDefault();
        close.current();
        trigger?.focus();
        return;
      }
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
      const buttons = Array.from(
        menu.current?.querySelectorAll<HTMLButtonElement>(
          "button:not(:disabled)",
        ) || [],
      );
      if (!buttons.length) return;
      event.preventDefault();
      const index = buttons.indexOf(
        document.activeElement as HTMLButtonElement,
      );
      buttons[
        event.key === "Home"
          ? 0
          : event.key === "End"
            ? buttons.length - 1
            : (index + (event.key === "ArrowDown" ? 1 : -1) + buttons.length) %
              buttons.length
      ]?.focus();
    };
    const observer = new ResizeObserver(update);
    if (anchor.current) observer.observe(anchor.current);
    if (menu.current) observer.observe(menu.current);
    addEventListener("resize", update);
    addEventListener("scroll", update, true);
    document.addEventListener("pointerdown", outside);
    document.addEventListener("keydown", key);
    return () => {
      cancelAnimationFrame(focusFrame);
      observer.disconnect();
      removeEventListener("resize", update);
      removeEventListener("scroll", update, true);
      document.removeEventListener("pointerdown", outside);
      document.removeEventListener("keydown", key);
    };
  }, [open]);
  return (
    <div ref={anchor} className="inline-flex">
      <Button
        variant="ghost"
        size="icon"
        aria-label={label}
        aria-expanded={open}
        aria-haspopup="menu"
        aria-controls={open ? id : undefined}
        onClick={onToggle}
        onKeyDown={(event) => {
          if (!open && event.key === "ArrowDown") {
            event.preventDefault();
            onToggle();
          }
        }}
      >
        <MoreHorizontal className="h-4 w-4" />
      </Button>
      {open &&
        createPortal(
          <div
            id={id}
            ref={menu}
            role="menu"
            aria-label={label}
            className="floating-action-menu"
            style={{
              left: position?.left ?? 0,
              top: position?.top ?? 0,
              visibility: position ? "visible" : "hidden",
            }}
          >
            {children}
          </div>,
          document.body,
        )}
    </div>
  );
}
