import { useEffect, useState, type FormEvent } from "react";
import { api, type NodeItem } from "@/lib/api";
import { useLocale, t } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input, Select, Textarea } from "@/components/ui/controls";
import { Dialog } from "@/components/common/dialog";
import { PageHeader } from "@/components/common/page-header";
type Body = {
  xray?: object;
  singbox?: object;
  snippet_ids?: number[];
  core?: string;
  config?: object;
};
type Item = {
  id?: number;
  name: string;
  note: string;
  revision: number;
  body?: Body;
  node_ids?: number[];
};
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));
function object(text: string) {
  const v = JSON.parse(text);
  if (!v || typeof v !== "object" || Array.isArray(v))
    throw Error(t("Enter a JSON object."));
  return v;
}
export function ProfilesPage() {
  useLocale();
  const [kind, setKind] = useState<"profiles" | "snippets">("profiles"),
    [items, setItems] = useState<Item[]>([]),
    [snippets, setSnippets] = useState<Item[]>([]),
    [nodes, setNodes] = useState<NodeItem[]>([]),
    [editor, setEditor] = useState<Item | null>(null),
    [first, setFirst] = useState("{}"),
    [second, setSecond] = useState("{}"),
    [assigned, setAssigned] = useState<Item | null>(null),
    [selected, setSelected] = useState<number[]>([]),
    [deleting, setDeleting] = useState<Item | null>(null),
    [history, setHistory] = useState<{
      item: Item;
      rows: { revision: number; created_at: string }[];
    } | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true);
  async function load(signal?: AbortSignal) {
    try {
      const [a, b, c] = await Promise.all([
        api<Item[]>(`/${kind}`, { signal }),
        api<Item[]>("/snippets", { signal }),
        api<NodeItem[]>("/nodes", { signal }),
      ]);
      if (!signal?.aborted) {
        setItems(a);
        setSnippets(b);
        setNodes(c);
        setLoading(false);
      }
    } catch (e) {
      if (!signal?.aborted) {
        setError(message(e));
        setLoading(false);
      }
    }
  }
  useEffect(() => {
    setLoading(true);
    setError("");
    const c = new AbortController();
    void load(c.signal);
    return () => c.abort();
  }, [kind]);
  async function open(item?: Item) {
    setBusy(true);
    setError("");
    try {
      const d = item
        ? await api<Item>(`/${kind}/${item.id}`)
        : {
            name: "",
            note: "",
            revision: 0,
            body:
              kind === "profiles"
                ? { xray: {}, singbox: {}, snippet_ids: [] }
                : { core: "xray", config: {} },
          };
      setEditor(d);
      setFirst(
        JSON.stringify(
          kind === "profiles" ? d.body?.xray : d.body?.config,
          null,
          2,
        ) || "{}",
      );
      setSecond(JSON.stringify(d.body?.singbox, null, 2) || "{}");
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!editor) return;
    setBusy(true);
    setError("");
    try {
      const body =
        kind === "profiles"
          ? {
              xray: object(first),
              singbox: object(second),
              snippet_ids: editor.body?.snippet_ids || [],
            }
          : { core: editor.body?.core || "xray", config: object(first) };
      await api(`/${kind}${editor.id ? `/${editor.id}` : ""}`, {
        method: editor.id ? "PATCH" : "POST",
        body: JSON.stringify({
          name: editor.name,
          note: editor.note,
          revision: editor.revision,
          body,
        }),
      });
      setEditor(null);
      await load();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function assign(item: Item) {
    setBusy(true);
    setError("");
    try {
      const d = await api<Item>(`/profiles/${item.id}`);
      setAssigned(d);
      setSelected(d.node_ids || []);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function versions(item: Item) {
    setBusy(true);
    setError("");
    try {
      setHistory({ item, rows: await api(`/${kind}/${item.id}/revisions`) });
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function execute(path: string, method: string, body?: object) {
    setBusy(true);
    setError("");
    try {
      await api(path, {
        method,
        body: body ? JSON.stringify(body) : undefined,
      });
      setAssigned(null);
      setDeleting(null);
      setHistory(null);
      await load();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-5">
      <PageHeader
        title="Config profiles"
        description=""
        actions={
          <Button disabled={busy} onClick={() => void open()}>
            {t(kind === "profiles" ? "Add profile" : "Add snippet")}
          </Button>
        }
      />
      <div className="flex gap-2">
        {(["profiles", "snippets"] as const).map((k) => (
          <Button
            key={k}
            variant={kind === k ? "secondary" : "outline"}
            aria-pressed={kind === k}
            onClick={() => setKind(k)}
          >
            {t(k === "profiles" ? "Profiles" : "Snippets")}
          </Button>
        ))}
      </div>
      <p className="text-[12px] text-[var(--muted)]">
        {t(
          "Share network policy across nodes. Changes deploy together. The last 50 revisions can be restored.",
        )}
      </p>
      {error && !editor && !assigned && !history && !deleting && (
        <p role="alert" className="text-red-500">
          {error}
        </p>
      )}
      {loading ? (
        <p role="status">{t("Loading…")}</p>
      ) : (
        <div className="divide-y divide-[var(--border)] rounded-md border border-[var(--border)]">
          {items.map((item) => (
            <article
              key={item.id}
              className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center"
            >
              <div className="min-w-0 flex-1">
                <h3 className="break-words text-[13px] font-medium">
                  {item.name}
                </h3>
                <p className="break-words text-[12px] text-[var(--muted)]">
                  {item.note} · {t("Revision")} {item.revision}
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => void open(item)}
                >
                  {t("Edit")}
                </Button>
                {kind === "profiles" && (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() => void assign(item)}
                  >
                    {t("Assign nodes")}
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => void versions(item)}
                >
                  {t("History")}
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={busy}
                  onClick={() => {
                    setError("");
                    setDeleting(item);
                  }}
                >
                  {t("Delete")}
                </Button>
              </div>
            </article>
          ))}
          {items.length === 0 && (
            <p className="p-4 text-[12px]">{t("No records yet")}</p>
          )}
        </div>
      )}
      {editor && (
        <Dialog
          label={t(kind === "profiles" ? "Profile" : "Snippet")}
          size="lg"
          onClose={() => !busy && setEditor(null)}
        >
          <form onSubmit={save} className="space-y-4">
            <h3>{t(kind === "profiles" ? "Profile" : "Snippet")}</h3>
            <label className="label">
              {t("Name")}
              <Input
                required
                maxLength={96}
                value={editor.name}
                onChange={(e) =>
                  setEditor((d) => d && { ...d, name: e.target.value })
                }
              />
            </label>
            <label className="label">
              {t("Note")}
              <Input
                value={editor.note}
                onChange={(e) =>
                  setEditor((d) => d && { ...d, note: e.target.value })
                }
              />
            </label>
            {kind === "snippets" && (
              <label className="label">
                {t("Core")}
                <Select
                  value={editor.body?.core || "xray"}
                  onChange={(e) =>
                    setEditor(
                      (d) =>
                        d && {
                          ...d,
                          body: { ...d.body, core: e.target.value },
                        },
                    )
                  }
                >
                  <option value="xray">Xray</option>
                  <option value="singbox">sing-box</option>
                </Select>
              </label>
            )}
            <label className="label">
              {t(kind === "profiles" ? "Xray policy JSON" : "Policy JSON")}
              <Textarea
                className="min-h-48 font-mono"
                dir="ltr"
                spellCheck={false}
                value={first}
                onChange={(e) => setFirst(e.target.value)}
              />
            </label>
            {kind === "profiles" && (
              <>
                <label className="label">
                  {t("sing-box policy JSON")}
                  <Textarea
                    className="min-h-48 font-mono"
                    dir="ltr"
                    spellCheck={false}
                    value={second}
                    onChange={(e) => setSecond(e.target.value)}
                  />
                </label>
                <fieldset className="space-y-2">
                  <legend>{t("Snippets")}</legend>
                  {snippets.map((s) => (
                    <label key={s.id} className="flex gap-2 text-[12px]">
                      <input
                        type="checkbox"
                        checked={
                          editor.body?.snippet_ids?.includes(s.id!) || false
                        }
                        onChange={() =>
                          setEditor((d) => {
                            if (!d) return d;
                            const ids = d.body?.snippet_ids || [];
                            return {
                              ...d,
                              body: {
                                ...d.body,
                                snippet_ids: ids.includes(s.id!)
                                  ? ids.filter((i) => i !== s.id)
                                  : [...ids, s.id!],
                              },
                            };
                          })
                        }
                      />
                      {s.name}
                    </label>
                  ))}
                </fieldset>
              </>
            )}
            <p className="text-[11px] text-[var(--muted)]">
              {t(
                "Allowed sections: DNS, routing, outbounds and logging; Xray also supports policy. Managed listeners and telemetry remain controlled by the panel.",
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
                {t("Save")}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
      {assigned && (
        <Dialog
          label={t("Assign nodes")}
          onClose={() => !busy && setAssigned(null)}
        >
          <div className="space-y-4">
            <h3>{assigned.name}</h3>
            {nodes.map((n) => (
              <label key={n.id} className="flex gap-2 text-[12px]">
                <input
                  type="checkbox"
                  checked={selected.includes(n.id)}
                  onChange={() =>
                    setSelected((ids) =>
                      ids.includes(n.id)
                        ? ids.filter((i) => i !== n.id)
                        : [...ids, n.id],
                    )
                  }
                />
                {n.name}
              </label>
            ))}
            {error && (
              <p role="alert" className="text-red-500">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setAssigned(null)}
              >
                {t("Cancel")}
              </Button>
              <Button
                disabled={busy}
                onClick={() =>
                  void execute(`/profiles/${assigned.id}/assign`, "POST", {
                    node_ids: selected,
                  })
                }
              >
                {t("Save")}
              </Button>
            </div>
          </div>
        </Dialog>
      )}
      {history && (
        <Dialog label={t("History")} onClose={() => !busy && setHistory(null)}>
          <div className="space-y-4">
            <h3>{history.item.name}</h3>
            {history.rows.map((row) => (
              <div
                key={row.revision}
                className="flex items-center justify-between gap-3 text-[12px]"
              >
                <span>
                  {t("Revision")} {row.revision} ·{" "}
                  {new Date(row.created_at).toLocaleString()}
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy || row.revision === history.item.revision}
                  onClick={() =>
                    void execute(
                      `/${kind}/${history.item.id}/restore`,
                      "POST",
                      { revision: row.revision },
                    )
                  }
                >
                  {t("Restore")}
                </Button>
              </div>
            ))}
            {error && (
              <p role="alert" className="text-red-500">
                {error}
              </p>
            )}
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => setHistory(null)}
            >
              {t("Close")}
            </Button>
          </div>
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
                onClick={() =>
                  void execute(`/${kind}/${deleting.id}`, "DELETE")
                }
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
