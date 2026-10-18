import { useEffect, useState } from "react";
import { Download, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { t } from "@/lib/i18n";
interface InstallEvent extends Event {
  prompt(): Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
}
export function PWAControls() {
  const [install, setInstall] = useState<InstallEvent | null>(null),
    [waiting, setWaiting] = useState<ServiceWorker | null>(null),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    if (
      !import.meta.env.PROD ||
      !window.isSecureContext ||
      !("serviceWorker" in navigator)
    )
      return;
    let active = true,
      requested = false,
      registration: ServiceWorkerRegistration | undefined,
      worker: ServiceWorker | null = null;
    const offered = (event: Event) => {
      event.preventDefault();
      if (!matchMedia("(display-mode: standalone)").matches)
        setInstall(event as InstallEvent);
    };
    const installed = () => setInstall(null);
    const changed = () => {
      if (requested) window.location.reload();
    };
    const state = () => {
      if (
        active &&
        worker?.state === "installed" &&
        navigator.serviceWorker.controller
      )
        setWaiting(registration?.waiting || worker);
    };
    const found = () => {
      worker?.removeEventListener("statechange", state);
      worker = registration?.installing || null;
      worker?.addEventListener("statechange", state);
    };
    const update = () => {
      requested = true;
    };
    window.addEventListener("beforeinstallprompt", offered);
    window.addEventListener("appinstalled", installed);
    window.addEventListener("veloray:pwa-update", update);
    navigator.serviceWorker.addEventListener("controllerchange", changed);
    void navigator.serviceWorker
      .register("/service-worker.js", { scope: "/", updateViaCache: "none" })
      .then((reg) => {
        if (!active) return;
        registration = reg;
        setWaiting(reg.waiting);
        reg.addEventListener("updatefound", found);
        found();
        void reg.update().catch(() => {});
      })
      .catch(() => {});
    return () => {
      active = false;
      window.removeEventListener("beforeinstallprompt", offered);
      window.removeEventListener("appinstalled", installed);
      window.removeEventListener("veloray:pwa-update", update);
      navigator.serviceWorker.removeEventListener("controllerchange", changed);
      registration?.removeEventListener("updatefound", found);
      worker?.removeEventListener("statechange", state);
    };
  }, []);
  return (
    <>
      {install && (
        <Button
          variant="ghost"
          size="icon"
          title={t("Install app")}
          disabled={busy}
          onClick={async () => {
            setBusy(true);
            try {
              await install.prompt();
              await install.userChoice;
              setInstall(null);
            } finally {
              setBusy(false);
            }
          }}
        >
          <Download className="h-4 w-4" />
        </Button>
      )}
      {waiting && (
        <Button
          variant="outline"
          size="sm"
          title={t("Update app")}
          onClick={() => {
            window.dispatchEvent(new Event("veloray:pwa-update"));
            waiting.postMessage({ type: "VELO_UPDATE" });
          }}
        >
          <RefreshCw className="h-4 w-4" />
          <span className="hidden sm:inline">{t("Update app")}</span>
        </Button>
      )}
    </>
  );
}
