import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
export type HostJob = {
  id: number;
  kind: string;
  target: string;
  state: "queued" | "running" | "succeeded" | "failed";
  error: string;
};
export type PanelUpdateStatus = {
  version: string;
  state: "unknown" | "up_to_date" | "available" | "verifying";
  installed_commit: string;
  latest_commit: string;
  managed: boolean;
  auto_update_enabled: boolean;
  ci_status: string;
  error: string;
  checked_at: string;
  job: HostJob | null;
};
export function usePanelUpdates(enabled: boolean) {
  const [status, setStatus] = useState<PanelUpdateStatus | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const request = useRef<AbortController | null>(null);
  const refresh = useCallback(
    async (force = false) => {
      if (!enabled) return;
      request.current?.abort();
      const controller = new AbortController();
      request.current = controller;
      setBusy(true);
      const timeout = setTimeout(() => controller.abort(), 20000);
      try {
        const data = await api<PanelUpdateStatus>(
          force ? "/updates/check" : "/updates/status",
          { method: force ? "POST" : "GET", signal: controller.signal },
        );
        if (!controller.signal.aborted) {
          setStatus(data);
          setError("");
        }
      } catch (e) {
        if (request.current === controller)
          setError(e instanceof Error ? e.message : "Update check unavailable");
      } finally {
        clearTimeout(timeout);
        if (request.current === controller) {
          setBusy(false);
          request.current = null;
        }
      }
    },
    [enabled],
  );
  useEffect(() => {
    if (!enabled) return;
    void refresh();
    const update = () => {
      if (!document.hidden) void refresh();
    };
    window.addEventListener("veloray:update-status", update);
    const timer = setInterval(update, 60000);
    return () => {
      clearInterval(timer);
      window.removeEventListener("veloray:update-status", update);
      request.current?.abort();
      request.current = null;
    };
  }, [enabled, refresh]);
  const running =
    status?.job?.state === "queued" || status?.job?.state === "running";
  useEffect(() => {
    if (!running) return;
    const timer = setInterval(() => void refresh(), 5000);
    return () => clearInterval(timer);
  }, [running, refresh]);
  return { status, error, busy, refresh, running };
}
