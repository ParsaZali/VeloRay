import { useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input, Select, Textarea } from "@/components/ui/controls";
import { Dialog } from "@/components/common/dialog";
type Endpoint = {
  id?: number;
  name: string;
  enabled: boolean;
  channel: string;
  events: string[];
  url?: string;
  signing_secret?: string;
};
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));
export function NotificationEndpoints() {
  useLocale();
  const [rows, setRows] = useState<Endpoint[]>([]),
    [editor, setEditor] = useState<Endpoint | null>(null),
    [deleting, setDeleting] = useState<Endpoint | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [status, setStatus] = useState("");
  async function load(signal?: AbortSignal) {
    try {
      const data = await api<Endpoint[]>("/webhooks/endpoints", { signal });
      if (!signal?.aborted) setRows(data);
    } catch (e) {
      if (!signal?.aborted) setError(message(e));
    }
  }
  useEffect(() => {
    const c = new AbortController();
    void load(c.signal);
    return () => c.abort();
  }, []);
  async function save(e: FormEvent) {
    e.preventDefault();
    e.stopPropagation();
    if (!editor) return;
    setBusy(true);
    setError("");
    try {
      const body = {
        name: editor.name,
        enabled: editor.enabled,
        channel: editor.channel,
        events: editor.events,
        url: editor.url,
        signing_secret: editor.signing_secret || "",
      };
      await api(`/webhooks/endpoints${editor.id ? `/${editor.id}` : ""}`, {
        method: editor.id ? "PATCH" : "POST",
        body: JSON.stringify(body),
      });
      setEditor(null);
      await load();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function action(
    row: Endpoint,
    which: "test" | "retry" | "edit" | "delete",
  ) {
    setBusy(true);
    setError("");
    setStatus("");
    try {
      const path = `/webhooks/endpoints/${row.id}`;
      if (which === "edit")
        setEditor({ ...(await api<Endpoint>(path)), signing_secret: "" });
      else if (which === "delete") {
        await api(path, { method: "DELETE" });
        setDeleting(null);
        await load();
      } else {
        await api(`${path}/${which}`, { method: "POST", body: "{}" });
        setStatus(t("Delivery queued"));
      }
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="space-y-3 sm:col-span-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-[13px] font-medium">
          {t("Notification destinations")}
        </h3>
        <Button
          variant="outline"
          onClick={() => {
            setError("");
            setEditor({
              name: "",
              enabled: false,
              channel: "webhook",
              events: [],
              url: "",
              signing_secret: "",
            });
          }}
        >
          {t("Add destination")}
        </Button>
      </div>
      <p className="text-[11px] text-[var(--muted)]">
        {t(
          "Route events independently to signed HTTPS endpoints or Discord. Empty filters receive all events; use client.*, node.* or an exact event name.",
        )}
      </p>
      {error && !editor && !deleting && (
        <p role="alert" className="text-red-500">
          {error}
        </p>
      )}
      {status && <p role="status">{status}</p>}
      {rows.map((row) => (
        <div
          key={row.id}
          className="flex flex-wrap items-center gap-2 rounded-md border border-[var(--border)] p-3"
        >
          <span className="min-w-0 flex-1 break-words text-[12px]">
            {row.name} · {row.channel} ·{" "}
            {t(row.enabled ? "Enabled" : "Disabled")}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => void action(row, "edit")}
          >
            {t("Edit")}
          </Button>
          {(["test", "retry"] as const).map((which) => (
            <Button
              key={which}
              size="sm"
              variant="outline"
              disabled={busy || !row.enabled}
              onClick={() => void action(row, which)}
            >
              {t(which === "test" ? "Queue test event" : "Retry failed")}
            </Button>
          ))}
          <Button
            size="sm"
            variant="danger"
            onClick={() => {
              setError("");
              setDeleting(row);
            }}
          >
            {t("Delete")}
          </Button>
        </div>
      ))}
      {editor && (
        <Dialog
          label={t("Notification destination")}
          onClose={() => !busy && setEditor(null)}
        >
          <form onSubmit={save} className="space-y-4">
            <h3>{t("Notification destination")}</h3>
            <label className="label">
              {t("Name")}
              <Input
                required
                maxLength={96}
                value={editor.name}
                onChange={(e) => setEditor({ ...editor, name: e.target.value })}
              />
            </label>
            <label className="label">
              {t("Channel")}
              <Select
                value={editor.channel}
                onChange={(e) =>
                  setEditor({ ...editor, channel: e.target.value })
                }
              >
                <option value="webhook">{t("Signed webhook")}</option>
                <option value="discord">Discord</option>
                <option value="email">{t("Email")}</option>
              </Select>
            </label>
            <label className="label">
              {t(
                editor.channel === "email"
                  ? "Recipient emails"
                  : "HTTPS destination URL",
              )}
              <Input
                type={editor.channel === "email" ? "text" : "url"}
                required
                value={editor.url || ""}
                onChange={(e) => setEditor({ ...editor, url: e.target.value })}
              />
            </label>
            {editor.channel === "webhook" && (
              <label className="label">
                {t("HMAC signing secret")}
                <Input
                  type="password"
                  autoComplete="new-password"
                  required={!editor.id}
                  minLength={32}
                  maxLength={1024}
                  placeholder={t("Leave blank to keep the stored secret")}
                  value={editor.signing_secret || ""}
                  onChange={(e) =>
                    setEditor({ ...editor, signing_secret: e.target.value })
                  }
                />
              </label>
            )}
            <label className="label">
              {t("Event filters")}
              <Textarea
                rows={4}
                value={editor.events.join("\n")}
                onChange={(e) =>
                  setEditor({
                    ...editor,
                    events: e.target.value
                      .split(/\r?\n/)
                      .map((v) => v.trim())
                      .filter(Boolean),
                  })
                }
                placeholder={"client.*\nnode.status"}
              />
            </label>
            <label className="flex gap-2 text-[12px]">
              <input
                type="checkbox"
                checked={editor.enabled}
                onChange={(e) =>
                  setEditor({ ...editor, enabled: e.target.checked })
                }
              />
              {t("Enabled")}
            </label>
            {error && (
              <p role="alert" className="text-red-500">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setEditor(null)}
              >
                {t("Cancel")}
              </Button>
              <Button type="submit" disabled={busy}>
                {t(busy ? "Saving…" : "Save")}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
      {deleting && (
        <Dialog label={t("Delete")} onClose={() => !busy && setDeleting(null)}>
          <div className="space-y-4">
            <p>
              {t("Delete")} {deleting.name}?{" "}
              {t(
                "Pending deliveries for this destination will also be removed.",
              )}
            </p>
            {error && (
              <p role="alert" className="text-red-500">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                disabled={busy}
                variant="outline"
                onClick={() => setDeleting(null)}
              >
                {t("Cancel")}
              </Button>
              <Button
                disabled={busy}
                variant="danger"
                onClick={() => void action(deleting, "delete")}
              >
                {t("Delete")}
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </section>
  );
}
