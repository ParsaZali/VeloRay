import { Dialog } from "@/components/common/dialog";
import { LoadingState } from "@/components/common/loading-state";
import { notify } from "@/lib/notifications";
import { formatDate } from "@/lib/dates";
import { Input } from "@/components/ui/controls";
import { t } from "@/lib/i18n";
import { useLocale, tx } from "@/lib/i18n";
import { FormEvent, useEffect, useState } from "react";
import {
  ChevronUp,
  Pencil,
  Plus,
  RefreshCw,
  Rocket,
  Server,
  Trash2,
  X,
} from "lucide-react";
import { api, type NodeItem } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/common/page-header";
import { NodeEnrollment } from "@/components/node-enrollment";

const initial = {
  name: "",
  public_host: "",
  agent_url: "https://",
  agent_token: "",
  agent_cert_sha256: "",
  verify_tls: true,
  enabled: true,
};

export function NodesPage() {
  useLocale();
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [form, setForm] = useState({ ...initial });
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<NodeItem | null>(null);
  const [msg, setMsg] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<number | null>(null);
  const [formBusy, setFormBusy] = useState(false);
  const load = () => api<NodeItem[]>("/nodes/").then(setNodes);
  useEffect(() => {
    load()
      .catch((e) => setMsg(e.message))
      .finally(() => setLoading(false));
  }, []);

  async function add(e: FormEvent) {
    e.preventDefault();
    if (formBusy) return;
    setFormBusy(true);
    setMsg("");
    try {
      await api("/nodes/", { method: "POST", body: JSON.stringify(form) });
      setForm({ ...initial });
      setOpen(false);
      notify(
        t("Remote node saved. Probe it before deploying traffic."),
        "success",
      );
      await load();
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setFormBusy(false);
    }
  }
  async function probe(n: NodeItem) {
    setBusy(n.id);
    setMsg("");
    try {
      await api(`/nodes/${n.id}/probe/`, { method: "POST" });
      notify(tx("{0} responded successfully.", n.name), "success");
      await load();
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(null);
    }
  }
  async function deploy(n: NodeItem) {
    setBusy(n.id);
    setMsg("");
    try {
      await api(`/nodes/${n.id}/deploy/`, { method: "POST" });
      notify(tx("Validated config deployed to {0}.", n.name), "success");
      await load();
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setBusy(null);
    }
  }
  async function remove(n: NodeItem) {
    if (n.is_local) return;
    if (
      !confirm(
        tx("Delete node {0}? Remove this node's inbounds first.", n.name),
      )
    )
      return;
    setMsg("");
    try {
      await api(`/nodes/${n.id}/`, { method: "DELETE" });
      notify(tx("{0} deleted.", n.name), "success");
      await load();
    } catch (e: any) {
      setMsg(e.message);
    }
  }
  function beginEdit(n: NodeItem) {
    if (n.is_local) return;
    setMsg("");
    setEditing(n);
    setForm({
      name: n.name,
      public_host: n.public_host,
      agent_url: n.agent_url,
      agent_token: "",
      agent_cert_sha256: n.agent_cert_sha256 || "",
      verify_tls: n.verify_tls,
      enabled: n.enabled,
    });
    setOpen(false);
  }
  async function saveEdit(e: FormEvent) {
    e.preventDefault();
    if (!editing || formBusy) return;
    setFormBusy(true);
    try {
      const payload: any = {
        name: form.name,
        public_host: form.public_host,
        agent_url: form.agent_url,
        verify_tls: form.verify_tls,
        agent_cert_sha256: form.agent_cert_sha256,
        enabled: form.enabled,
      };
      if (form.agent_token) payload.agent_token = form.agent_token;
      await api(`/nodes/${editing.id}/`, {
        method: "PATCH",
        body: JSON.stringify(payload),
      });
      setEditing(null);
      setForm({ ...initial });
      notify(t("Remote node updated."), "success");
      await load();
    } catch (e: any) {
      setMsg(e.message);
    } finally {
      setFormBusy(false);
    }
  }
  function closeEditor() {
    setEditing(null);
    setForm({ ...initial });
  }

  const formBlock = (editingMode: boolean) => (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>
            {editingMode ? t("Edit remote node") : t("Remote node")}
          </CardTitle>
        </div>
        {
          <Button
            variant="ghost"
            size="icon"
            aria-label={t("Close")}
            disabled={formBusy}
            onClick={() => {
              setOpen(false);
              closeEditor();
            }}
          >
            <X className="h-4 w-4" />
          </Button>
        }
      </CardHeader>
      <CardContent>
        <form
          onSubmit={editingMode ? saveEdit : add}
          className="grid gap-4 sm:grid-cols-2"
        >
          <label className="label">
            {t("Node name")}
            <Input
              className="input"
              placeholder={t("Frankfurt 01")}
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              required
            />
          </label>
          <label className="label">
            {t(
              "Certificate SHA-256 (optional)",
              "اثر انگشت SHA-256 گواهی (اختیاری)",
            )}
            <Input
              dir="ltr"
              maxLength={64}
              value={form.agent_cert_sha256}
              onChange={(e) =>
                setForm({
                  ...form,
                  agent_cert_sha256: e.target.value.toLowerCase(),
                })
              }
            />
          </label>
          <label className="label">
            {t("Public host")}
            <Input
              className="input"
              placeholder={t("node.example.com")}
              dir="ltr"
              value={form.public_host}
              onChange={(e) =>
                setForm({ ...form, public_host: e.target.value })
              }
              required
            />
          </label>
          <label className="label">
            {t("Agent URL")}
            <Input
              className="input"
              placeholder={t("https://node.example.com:9191")}
              dir="ltr"
              value={form.agent_url}
              onChange={(e) => setForm({ ...form, agent_url: e.target.value })}
              required
            />
          </label>
          <label className="label">
            {t("Agent token")}
            <Input
              className="input mono"
              type="password"
              placeholder={
                editingMode ? t("Unchanged when empty") : t("Paste token")
              }
              value={form.agent_token}
              onChange={(e) =>
                setForm({ ...form, agent_token: e.target.value })
              }
              required={!editingMode}
            />
          </label>
          <label className="col-span-full flex items-center gap-2 text-[11px] text-[var(--muted-strong)]">
            <Input
              type="checkbox"
              checked={form.verify_tls}
              onChange={(e) =>
                setForm({ ...form, verify_tls: e.target.checked })
              }
            />
            {t("Verify the Agent TLS certificate")}
          </label>
          {msg && (
            <div className="workspace-error col-span-full" role="alert">
              {msg}
            </div>
          )}
          <div className="dialog-actions inbound-actions col-span-full flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={formBusy}
              onClick={() => {
                setOpen(false);
                closeEditor();
              }}
            >
              {t("Cancel")}
            </Button>
            <Button type="submit" disabled={formBusy}>
              {formBusy ? (
                "Saving…"
              ) : editingMode ? (
                t("Save changes")
              ) : (
                <>
                  <Plus className="h-3.5 w-3.5" />
                  {t("Save node")}
                </>
              )}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Nodes")}
        description=""
        actions={
          <>
            <NodeEnrollment />
            <Button
              variant="outline"
              onClick={() => {
                setMsg("");
                setOpen(!open);
                setEditing(null);
                setForm({ ...initial });
              }}
            >
              {open ? (
                <ChevronUp className="h-3.5 w-3.5" />
              ) : (
                <Plus className="h-3.5 w-3.5" />
              )}
              {open ? t("Close") : t("Add remote node")}
            </Button>
          </>
        }
      />
      {msg && (
        <div role="alert" className="workspace-error">
          {msg}
        </div>
      )}
      {(open || editing) && (
        <Dialog
          label={editing ? t("Edit remote node") : t("Remote node")}
          padded={false}
          size="lg"
          className="node-editor"
          onClose={() => {
            if (!formBusy) {
              setOpen(false);
              closeEditor();
            }
          }}
        >
          {formBlock(Boolean(editing))}
        </Dialog>
      )}
      {loading ? (
        <LoadingState rows={3} />
      ) : (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <CardTitle>{t("Fleet")}</CardTitle>
              <Badge>{nodes.length}</Badge>
            </div>
          </CardHeader>
          <CardContent className="p-0">
            {!nodes.length && (
              <div className="empty-state">
                <Server size={28} className="mx-auto mb-3" />
                <p>{t("No nodes registered")}</p>
              </div>
            )}
            <div className="hidden md:block table-wrap">
              <table className="data-table min-w-[740px]">
                <thead>
                  <tr>
                    <th>{t("Node")}</th>
                    <th>{t("Public host")}</th>
                    <th>{t("Status")}</th>
                    <th>{t("Xray")}</th>
                    <th>{t("Last seen")}</th>
                    <th className="text-right">{t("Actions")}</th>
                  </tr>
                </thead>
                <tbody>
                  {nodes.map((n) => (
                    <tr key={n.id}>
                      <td>
                        <div className="flex min-w-0 items-center gap-2">
                          <div className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-[var(--border)] bg-[var(--surface-2)]">
                            <Server className="h-3.5 w-3.5" />
                          </div>
                          <div className="min-w-0">
                            <div className="flex items-center gap-1.5">
                              <span className="truncate font-semibold text-[var(--fg)]">
                                {n.name}
                              </span>
                              {n.is_local && (
                                <Badge tone="blue">{t("Panel server")}</Badge>
                              )}
                            </div>
                          </div>
                        </div>
                      </td>
                      <td>
                        <span className="truncate-cell max-w-52">
                          {n.public_host}
                        </span>
                      </td>
                      <td>
                        <Badge
                          tone={
                            n.status === "ok"
                              ? "green"
                              : n.status === "offline"
                                ? "red"
                                : "amber"
                          }
                        >
                          {n.status}
                        </Badge>
                        {n.last_deploy_error && (
                          <p className="mt-1 max-w-52 break-words text-[11px] text-red-500">
                            {n.last_deploy_error}
                          </p>
                        )}
                      </td>
                      <td>
                        <span className="truncate-cell max-w-40">
                          {n.xray_version || "—"}
                        </span>
                      </td>
                      <td>
                        {n.last_seen_at
                          ? formatDate(n.last_seen_at, {
                              hour: "2-digit",
                              minute: "2-digit",
                            })
                          : t("Never")}
                      </td>
                      <td>
                        <div className="flex justify-end gap-1">
                          <Button
                            title={t("Probe")}
                            variant="ghost"
                            size="icon"
                            disabled={busy === n.id}
                            onClick={() => probe(n)}
                          >
                            <RefreshCw
                              className={`h-3.5 w-3.5 ${busy === n.id ? "animate-spin" : ""}`}
                            />
                          </Button>
                          <Button
                            title={t("Deploy")}
                            variant="outline"
                            size="icon"
                            disabled={busy === n.id}
                            onClick={() => deploy(n)}
                          >
                            <Rocket className="h-3.5 w-3.5" />
                          </Button>
                          {!n.is_local && (
                            <>
                              <Button
                                title={t("Edit")}
                                variant="ghost"
                                size="icon"
                                onClick={() => beginEdit(n)}
                              >
                                <Pencil className="h-3.5 w-3.5" />
                              </Button>
                              <Button
                                title={t("Delete")}
                                variant="ghost"
                                size="icon"
                                onClick={() => remove(n)}
                              >
                                <Trash2 className="h-3.5 w-3.5" />
                              </Button>
                            </>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="divide-y divide-[var(--border-subtle)] md:hidden">
              {nodes.map((n) => (
                <div key={n.id} className="p-4">
                  <div className="flex items-start gap-3">
                    <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg border border-[var(--border)] bg-[var(--surface-2)]">
                      <Server className="h-4 w-4" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-semibold">{n.name}</span>
                        {n.is_local && <Badge tone="blue">{t("Local")}</Badge>}
                        <Badge
                          tone={
                            n.status === "ok"
                              ? "green"
                              : n.status === "offline"
                                ? "red"
                                : "amber"
                          }
                        >
                          {n.status}
                        </Badge>
                      </div>
                      <div className="mt-1 break-all text-[10px] text-[var(--muted)]">
                        {n.public_host}
                      </div>
                    </div>
                  </div>
                  <div className="mt-3 grid grid-cols-2 gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={busy === n.id}
                      onClick={() => probe(n)}
                    >
                      <RefreshCw className="h-3.5 w-3.5" />
                      {t("Probe")}
                    </Button>
                    <Button
                      size="sm"
                      disabled={busy === n.id}
                      onClick={() => deploy(n)}
                    >
                      <Rocket className="h-3.5 w-3.5" />
                      {t("Deploy")}
                    </Button>
                    {!n.is_local && (
                      <>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => beginEdit(n)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                          {t("Edit")}
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => remove(n)}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                          {t("Delete")}
                        </Button>
                      </>
                    )}
                  </div>
                </div>
              ))}
            </div>
            {!nodes.length && (
              <div className="p-10 text-center text-[12px] text-[var(--muted)]">
                {t(
                  "No nodes yet. Add a remote node or check the local installation.",
                )}
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
