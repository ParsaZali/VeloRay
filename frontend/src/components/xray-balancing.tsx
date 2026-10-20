import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input, Select, Textarea } from "@/components/ui/controls";

export type Balancer = {
  tag: string;
  selector: string[];
  fallbackTag?: string;
  strategy?: { type?: string; settings?: Record<string, unknown> };
};
export type BalancingState = {
  node: number;
  tags: string[];
  outboundTags: string[];
};
type BalancingView = {
  balancers: Balancer[];
  outbound_tags: string[];
  revision: string;
  observatory_configured: boolean;
};

export function XrayBalancing({
  node,
  onGroupsChange,
}: {
  node: number;
  onGroupsChange: (value: BalancingState) => void;
}) {
  useLocale();
  const [view, setView] = useState<BalancingView | null>(null);
  const [draft, setDraft] = useState<Balancer[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [reload, setReload] = useState(0);
  const [saved, setSaved] = useState(false);
  const saveController = useRef<AbortController | null>(null);
  useEffect(() => () => saveController.current?.abort(), []);
  useEffect(() => {
    const controller = new AbortController();
    setView(null);
    setError("");
    onGroupsChange({ node, tags: [], outboundTags: [] });
    api<BalancingView>(`/nodes/${node}/balancing`, {
      signal: controller.signal,
    })
      .then((data) => {
        if (controller.signal.aborted) return;
        setView(data);
        setDraft(structuredClone(data.balancers));
        onGroupsChange({
          node,
          tags: data.balancers.map((group) => group.tag),
          outboundTags: data.outbound_tags,
        });
      })
      .catch((e) => {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : String(e));
      });
    return () => controller.abort();
  }, [node, reload, onGroupsChange]);
  function update(index: number, patch: Partial<Balancer>) {
    setSaved(false);
    setDraft((current) =>
      current.map((row, i) => (i === index ? { ...row, ...patch } : row)),
    );
  }
  async function save() {
    if (!view || busy) return;
    setBusy(true);
    setError("");
    setSaved(false);
    const controller = new AbortController();
    saveController.current = controller;
    try {
      const data = await api<BalancingView>(`/nodes/${node}/balancing`, {
        method: "PUT",
        signal: controller.signal,
        body: JSON.stringify({
          balancers: draft.map((group) => ({
            ...group,
            selector: group.selector.filter(Boolean),
          })),
          revision: view.revision,
        }),
      });
      if (controller.signal.aborted) return;
      setView(data);
      setDraft(structuredClone(data.balancers));
      setSaved(true);
      onGroupsChange({
        node,
        tags: data.balancers.map((group) => group.tag),
        outboundTags: data.outbound_tags,
      });
    } catch (e) {
      if (!controller.signal.aborted)
        setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return (
    <details className="rounded-lg border border-[var(--border)] p-4">
      <summary className="cursor-pointer font-medium text-sm">
        {t("Xray load balancing")}
      </summary>
      <div className="mt-4 space-y-4">
        <p className="text-xs text-[var(--muted)]">
          {t(
            "Selectors match outbound tag prefixes. New matching tags join automatically. Saving deploys this node and overrides inherited balancers.",
          )}
        </p>
        <p className="text-xs text-[var(--muted)]">
          {t(
            "Random and round robin distribute connections without checking availability. Least ping and least load require an observatory configured in the node or shared profile.",
          )}
        </p>
        {!view && !error && <p role="status">{t("Loading…")}</p>}
        {error && (
          <p role="alert" className="form-error">
            {error}
          </p>
        )}
        {saved && <p role="status">{t("Balancing saved and deployed.")}</p>}
        {view && (
          <fieldset disabled={busy} className="space-y-4">
            {draft.map((row, index) => {
              const matches = view.outbound_tags.filter((tag) =>
                row.selector.some((prefix) => prefix && tag.startsWith(prefix)),
              );
              return (
                <fieldset
                  key={index}
                  className="grid gap-3 sm:grid-cols-2 border border-[var(--border)] rounded-lg p-3"
                >
                  <legend className="text-xs px-1">
                    {t("Balancer")} {index + 1}
                  </legend>
                  <label className="label">
                    {t("Balancer tag")}
                    <Input
                      className="input"
                      dir="ltr"
                      maxLength={96}
                      value={row.tag}
                      onChange={(e) => update(index, { tag: e.target.value })}
                    />
                  </label>
                  <label className="label">
                    {t("Balancing strategy")}
                    <Select
                      className="input"
                      value={(row.strategy?.type || "random").toLowerCase()}
                      onChange={(e) =>
                        update(index, { strategy: { type: e.target.value } })
                      }
                    >
                      <option value="random">{t("Random")}</option>
                      <option value="roundrobin">{t("Round robin")}</option>
                      <option
                        value="leastping"
                        disabled={!view.observatory_configured}
                      >
                        {t("Least ping")}
                      </option>
                      <option
                        value="leastload"
                        disabled={!view.observatory_configured}
                      >
                        {t("Least load")}
                      </option>
                    </Select>
                  </label>
                  <label className="label">
                    {t("Selector prefixes")}
                    <Textarea
                      className="input min-h-20"
                      dir="ltr"
                      value={row.selector.join("\n")}
                      onChange={(e) =>
                        update(index, {
                          selector: e.target.value
                            .split(/\r?\n/)
                            .map((value) => value.trim()),
                        })
                      }
                    />
                  </label>
                  <label className="label">
                    {t("Fallback outbound")}
                    <Select
                      className="input"
                      value={row.fallbackTag || ""}
                      onChange={(e) =>
                        update(index, { fallbackTag: e.target.value })
                      }
                    >
                      <option value="">{t("None")}</option>
                      {view.outbound_tags.map((tag) => (
                        <option key={tag} value={tag}>
                          {tag}
                        </option>
                      ))}
                    </Select>
                  </label>
                  <p
                    className="sm:col-span-2 text-xs text-[var(--muted)]"
                    dir="ltr"
                  >
                    {t("Matching outbounds")}: {matches.join(", ") || t("None")}
                  </p>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      setDraft((current) =>
                        current.filter((_, i) => i !== index),
                      );
                      setSaved(false);
                    }}
                  >
                    {t("Remove balancer")}
                  </Button>
                </fieldset>
              );
            })}
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={draft.length >= 64}
                onClick={() => {
                  setDraft((current) => [
                    ...current,
                    { tag: "", selector: [], strategy: { type: "random" } },
                  ]);
                  setSaved(false);
                }}
              >
                {t("Add balancer")}
              </Button>
              <Button type="button" onClick={() => void save()}>
                {busy ? t("Saving…") : t("Save balancing & deploy")}
              </Button>
            </div>
          </fieldset>
        )}
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onClick={() => {
            setReload((current) => current + 1);
            setSaved(false);
          }}
        >
          {t("Reload balancing")}
        </Button>
      </div>
    </details>
  );
}
