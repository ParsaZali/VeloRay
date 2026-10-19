import { useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input, Select, Textarea } from "@/components/ui/controls";
import { Dialog } from "@/components/common/dialog";
import { PageHeader } from "@/components/common/page-header";

type Body = {
  url: string;
  links: string[];
  apps: string[];
  default_format: string;
  templates: Record<string, string>;
};
type Policy = {
  id?: number;
  name: string;
  enabled: boolean;
  customer_ids: number[];
  service_ids: number[];
  cache_count?: number;
  cached_at?: string;
  body?: Body;
};
type Target = { id: number; name: string };
const initial = (): Policy => ({
  name: "",
  enabled: true,
  customer_ids: [],
  service_ids: [],
  body: { url: "", links: [], apps: [], default_format: "auto", templates: {} },
});
const flip = (ids: number[], id: number) =>
  ids.includes(id) ? ids.filter((i) => i !== id) : [...ids, id];
const errorText = (e: unknown) => (e instanceof Error ? e.message : String(e));
export function SubscriptionPoliciesPage() {
  useLocale();
  const [rows, setRows] = useState<Policy[]>([]),
    [customers, setCustomers] = useState<Target[]>([]),
    [services, setServices] = useState<Target[]>([]),
    [editor, setEditor] = useState<Policy | null>(null),
    [deleting, setDeleting] = useState<Policy | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [query, setQuery] = useState("");
  async function load(signal?: AbortSignal) {
    try {
      const [a, b, c] = await Promise.all([
        api<Policy[]>("/subscription-policies", { signal }),
        api<Target[]>("/customers", { signal }),
        api<Target[]>("/services", { signal }),
      ]);
      if (!signal?.aborted) {
        setRows(a);
        setCustomers(b);
        setServices(c);
      }
    } catch (e) {
      if (!signal?.aborted) setError(errorText(e));
    }
  }
  useEffect(() => {
    const c = new AbortController();
    void load(c.signal);
    return () => c.abort();
  }, []);
  function body(key: keyof Body, value: unknown) {
    setEditor((d) => d && { ...d, body: { ...d.body!, [key]: value } });
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!editor) return;
    setBusy(true);
    setError("");
    try {
      await api(`/subscription-policies${editor.id ? `/${editor.id}` : ""}`, {
        method: editor.id ? "PATCH" : "POST",
        body: JSON.stringify({
          name: editor.name,
          enabled: editor.enabled,
          customer_ids: editor.customer_ids,
          service_ids: editor.service_ids,
          body: editor.body,
        }),
      });
      setEditor(null);
      await load();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-5">
      <PageHeader
        title="Subscription policies"
        description=""
        actions={
          <Button
            onClick={() => {
              setError("");
              setEditor(initial());
            }}
          >
            {t("Add policy")}
          </Button>
        }
      />
      <p className="text-[12px] text-[var(--muted)]">
        {t(
          "Combine local connections with external links. Policies can override export templates and defaults for assigned customers, services and apps.",
        )}
      </p>
      <Input
        aria-label={t("Search")}
        placeholder={t("Search")}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {error && !editor && !deleting && (
        <p role="alert" className="text-red-500">
          {error}
        </p>
      )}
      <div className="divide-y divide-[var(--border)] rounded-md border border-[var(--border)]">
        {rows
          .filter((i) => i.name.toLowerCase().includes(query.toLowerCase()))
          .map((item) => (
            <article
              key={item.id}
              className="flex flex-wrap items-center gap-3 p-4"
            >
              <div className="min-w-0 flex-1">
                <h3 className="break-words text-[13px] font-medium">
                  {item.name}
                </h3>
                <p className="text-[11px] text-[var(--muted)]">
                  {t(item.enabled ? "Enabled" : "Disabled")} ·{" "}
                  {item.cache_count || 0} {t("Cached connections")}
                </p>
              </div>
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={async () => {
                  setBusy(true);
                  setError("");
                  try {
                    await api(`/subscription-policies/${item.id}/refresh`, {
                      method: "POST",
                      body: "{}",
                    });
                    await load();
                  } catch (e) {
                    setError(errorText(e));
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                {t("Refresh source")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                aria-label={`${t("Edit")} ${item.name}`}
                onClick={async () => {
                  setBusy(true);
                  setError("");
                  try {
                    setEditor(
                      await api<Policy>(`/subscription-policies/${item.id}`),
                    );
                  } catch (e) {
                    setError(errorText(e));
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                {t("Edit")}
              </Button>
              <Button
                size="sm"
                variant="danger"
                onClick={() => {
                  setError("");
                  setDeleting(item);
                }}
              >
                {t("Delete")}
              </Button>
            </article>
          ))}
        {!rows.length && (
          <p className="p-4 text-[12px]">{t("No records yet")}</p>
        )}
      </div>
      {editor && (
        <Dialog
          label={t("Subscription policy")}
          size="lg"
          onClose={() => !busy && setEditor(null)}
        >
          <form onSubmit={save} className="space-y-4">
            <h3>{t("Subscription policy")}</h3>
            <label className="label">
              {t("Name")}
              <Input
                required
                maxLength={96}
                value={editor.name}
                onChange={(e) => setEditor({ ...editor, name: e.target.value })}
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
            <label className="label">
              {t("External feed URL")}
              <Input
                type="url"
                value={editor.body!.url}
                placeholder="https://provider.example/sub/..."
                onChange={(e) => body("url", e.target.value)}
              />
              <span>
                {t(
                  "Public HTTPS only. Refresh explicitly after saving; failed refreshes retain the previous cache.",
                )}
              </span>
            </label>
            <label className="label">
              {t("External connection links")}
              <Textarea
                rows={5}
                value={editor.body!.links.join("\n")}
                onChange={(e) =>
                  body(
                    "links",
                    e.target.value.split(/\r?\n/).filter((i) => i.trim()),
                  )
                }
                placeholder="vless://..."
              />
            </label>
            <div className="grid gap-4 sm:grid-cols-2">
              {(
                [
                  ["customer_ids", "Customers", customers],
                  ["service_ids", "Services and plans", services],
                ] as const
              ).map(([key, label, targets]) => (
                <fieldset key={key} className="space-y-2">
                  <legend>{t(label)}</legend>
                  <div className="max-h-44 space-y-2 overflow-auto rounded-md border border-[var(--border)] p-3">
                    {targets.map((i) => (
                      <label key={i.id} className="flex gap-2 text-[12px]">
                        <input
                          type="checkbox"
                          checked={editor[key].includes(i.id)}
                          onChange={() =>
                            setEditor({
                              ...editor,
                              [key]: flip(editor[key], i.id),
                            })
                          }
                        />
                        {i.name}
                      </label>
                    ))}
                  </div>
                </fieldset>
              ))}
            </div>
            <fieldset className="space-y-2">
              <legend>{t("Client applications")}</legend>
              <p className="text-[11px] text-[var(--muted)]">
                {t(
                  "No app selection applies to every app. Application matching controls presentation, not authorization.",
                )}
              </p>
              <div className="flex flex-wrap gap-3">
                {[
                  "sing-box",
                  "singbox",
                  "clash",
                  "mihomo",
                  "stash",
                  "hiddify",
                  "v2rayng",
                  "v2rayn",
                  "browser",
                  "other",
                ].map((app) => (
                  <label key={app} className="flex gap-2 text-[12px]">
                    <input
                      type="checkbox"
                      checked={editor.body!.apps.includes(app)}
                      onChange={() =>
                        body(
                          "apps",
                          editor.body!.apps.includes(app)
                            ? editor.body!.apps.filter((i) => i !== app)
                            : [...editor.body!.apps, app],
                        )
                      }
                    />
                    {app}
                  </label>
                ))}
              </div>
            </fieldset>
            <label className="label">
              {t("Default subscription format")}
              <Select
                value={editor.body!.default_format}
                onChange={(e) => body("default_format", e.target.value)}
              >
                {["auto", "base64", "raw", "clash", "singbox", "xray"].map(
                  (i) => (
                    <option key={i} value={i}>
                      {i}
                    </option>
                  ),
                )}
              </Select>
            </label>
            <details>
              <summary className="cursor-pointer text-[12px]">
                {t("Client templates")}
              </summary>
              <div className="mt-3 space-y-3">
                {["clash", "singbox", "xray"].map((format) => (
                  <label className="label" key={format}>
                    {format}
                    <Textarea
                      rows={4}
                      dir="ltr"
                      value={editor.body!.templates[format] || ""}
                      onChange={(e) =>
                        body("templates", {
                          ...editor.body!.templates,
                          [format]: e.target.value,
                        })
                      }
                    />
                  </label>
                ))}
              </div>
            </details>
            <p className="text-[11px] text-[var(--muted)]">
              {t(
                "External traffic and expiry are enforced by the external provider. Local account eligibility and HWID checks still protect this subscription.",
              )}
            </p>
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
              {t("Delete")} {deleting.name}?
            </p>
            {error && (
              <p role="alert" className="text-red-500">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setDeleting(null)}
              >
                {t("Cancel")}
              </Button>
              <Button
                variant="danger"
                disabled={busy}
                onClick={async () => {
                  setBusy(true);
                  try {
                    await api(`/subscription-policies/${deleting.id}`, {
                      method: "DELETE",
                    });
                    setDeleting(null);
                    await load();
                  } catch (e) {
                    setError(errorText(e));
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                {t("Delete")}
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </div>
  );
}
