import { createPortal } from "react-dom";
import { useLocale } from "@/lib/i18n";
import { useLayoutEffect, useRef, type ReactNode } from "react";
let modalLocks = 0;
let bodyOverflow = "";
export function Dialog({
  label,
  onClose,
  children,
  padded = true,
  size = "md",
  className = "",
}: {
  label: string;
  onClose: () => void;
  children: ReactNode;
  padded?: boolean;
  size?: "sm" | "md" | "lg";
  className?: string;
}) {
  useLocale();
  const root = useRef<HTMLDivElement>(null),
    close = useRef(onClose);
  close.current = onClose;
  useLayoutEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const el = root.current;
    if (!el) return;
    if (modalLocks === 0) bodyOverflow = document.body.style.overflow;
    modalLocks++;
    document.body.style.overflow = "hidden";
    const focusables = () =>
      Array.from(
        el.querySelectorAll<HTMLElement>(
          'button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),a[href],[tabindex="0"]',
        ),
      ).filter((x) => x.offsetParent !== null);
    (focusables()[0] || el).focus();
    const key = (e: KeyboardEvent) => {
      if (
        document
          .querySelectorAll(".modal-backdrop")
          .item(document.querySelectorAll(".modal-backdrop").length - 1) !==
        el.parentElement
      )
        return;
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        close.current();
      }
      if (e.key === "Tab") {
        const items = focusables(),
          first = items[0],
          last = items.at(-1);
        if (!first) {
          e.preventDefault();
          return;
        }
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last?.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("keydown", key);
      modalLocks--;
      if (modalLocks === 0) document.body.style.overflow = bodyOverflow;
      previous?.focus();
    };
  }, []);
  return createPortal(
    <div
      className="modal-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) close.current();
      }}
    >
      <div
        ref={root}
        className={`modal-panel modal-${size} ${padded ? "modal-padded" : ""} ${className}`}
        role="dialog"
        aria-modal="true"
        aria-label={label}
        tabIndex={-1}
      >
        {children}
      </div>
    </div>,
    document.body,
  );
}
