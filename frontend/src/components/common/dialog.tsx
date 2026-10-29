import { createPortal } from "react-dom";
import { useLocale } from "@/lib/i18n";
import { useRef, type ReactNode } from "react";
import { useModalFocus } from "./use-modal-focus";
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
  const root = useRef<HTMLDivElement>(null);
  useModalFocus(root, true, onClose);
  return createPortal(
    <div
      className="modal-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
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
