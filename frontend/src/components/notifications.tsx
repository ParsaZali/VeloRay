import { createPortal } from "react-dom";
import { useEffect, useRef, useState } from "react";
import { CheckCircle2, AlertCircle, Info, X } from "lucide-react";
import { t } from "@/lib/i18n";
import type { Notice } from "@/lib/notifications";
export function Notifications() {
  const [notice, setNotice] = useState<Notice | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => {
    const receive = (event: Event) => {
      clearTimeout(timer.current);
      setNotice((event as CustomEvent<Notice>).detail);
      timer.current = setTimeout(() => setNotice(null), 8000);
    };
    window.addEventListener("veloray:notice", receive);
    return () => {
      window.removeEventListener("veloray:notice", receive);
      clearTimeout(timer.current);
    };
  }, []);
  if (!notice) return null;
  const Icon =
    notice.tone === "success"
      ? CheckCircle2
      : notice.tone === "error"
        ? AlertCircle
        : Info;
  return createPortal(
    <div
      className={`panel-notice notice-${notice.tone}`}
      role={notice.tone === "error" ? "alert" : "status"}
    >
      <Icon size={19} aria-hidden="true" />
      <p>{notice.message}</p>
      <button
        type="button"
        aria-label={t("Close")}
        onClick={() => {
          clearTimeout(timer.current);
          setNotice(null);
        }}
      >
        <X size={16} />
      </button>
    </div>,
    document.body,
  );
}
