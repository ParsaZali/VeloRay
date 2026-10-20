import { useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { formatDate } from "@/lib/dates";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/controls";

export type GeoEntry = { tag: string; reference: string; count: number };
type GeoAsset = {
  kind: string;
  sha256: string;
  size: number;
  modified: string;
  total: number;
  matched: number;
  entries: GeoEntry[];
};

export function GeoAssets({
  node,
  onUse,
}: {
  node: number;
  onUse?: (entry: GeoEntry, kind: string) => void;
}) {
  useLocale();
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState("geosite");
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState({ query: "", revision: 0 });
  const [asset, setAsset] = useState<GeoAsset | null>(null);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");
  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    setAsset(null);
    setError("");
    setCopied("");
    const params = new URLSearchParams({ kind, q: search.query, limit: "100" });
    api<GeoAsset>(`/nodes/${node}/geo-assets?${params}`, {
      signal: controller.signal,
    })
      .then((data) => {
        if (!controller.signal.aborted) setAsset(data);
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : String(e));
      });
    return () => controller.abort();
  }, [node, kind, open, search]);
  function submit(event: FormEvent) {
    event.preventDefault();
    setSearch((current) => ({
      query: query.trim(),
      revision: current.revision + 1,
    }));
  }
  async function copy(reference: string) {
    try {
      await navigator.clipboard.writeText(reference);
      setCopied(reference);
    } catch {
      setError(t("Could not copy. Select the reference and copy it manually."));
    }
  }
  return (
    <details
      className="rounded-lg border border-[var(--border)] p-4"
      onToggle={(event) => setOpen(event.currentTarget.open)}
    >
      <summary className="cursor-pointer font-medium text-sm">
        {t("Installed GeoIP and Geosite lists")}
      </summary>
      {open && (
        <div className="mt-4 space-y-4">
          <p className="text-xs text-[var(--muted)]">
            {t(
              "Browse the assets installed on this node. Search by tag, then copy a reference or create an Xray rule. Assets are read only.",
            )}
          </p>
          <form className="flex flex-wrap items-end gap-3" onSubmit={submit}>
            <label className="space-y-1 text-sm">
              <span>{t("Asset")}</span>
              <Select
                value={kind}
                onChange={(event) => setKind(event.target.value)}
              >
                <option value="geosite">Geosite</option>
                <option value="geoip">GeoIP</option>
              </Select>
            </label>
            <label className="min-w-40 flex-1 space-y-1 text-sm">
              <span>{t("Search tags")}</span>
              <Input
                value={query}
                maxLength={96}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="ir, private, category-ads"
              />
            </label>
            <Button type="submit">{t("Search")}</Button>
          </form>
          {error ? (
            <p role="alert" className="text-sm text-[var(--danger)]">
              {error}
            </p>
          ) : !asset ? (
            <p role="status">{t("Loading…")}</p>
          ) : (
            <>
              <p className="text-xs text-[var(--muted)]">
                {t("Matches")}: {asset.matched} / {asset.total} · {t("Showing")}
                : {asset.entries.length} · {formatDate(asset.modified)} ·{" "}
                {(asset.size / 1048576).toFixed(1)} MiB
              </p>
              <details className="text-xs">
                <summary className="cursor-pointer">SHA-256</summary>
                <code className="break-all">{asset.sha256}</code>
              </details>
              {asset.matched === 0 ? (
                <p role="status">{t("No matching tags.")}</p>
              ) : (
                <ul className="max-h-80 divide-y divide-[var(--border)] overflow-auto">
                  {asset.entries.map((entry) => (
                    <li
                      key={entry.tag}
                      className="flex flex-wrap items-center gap-2 py-2 text-sm"
                    >
                      <code className="min-w-0 flex-1 break-all" dir="ltr">
                        {entry.reference}
                      </code>
                      <span className="text-xs text-[var(--muted)]">
                        {entry.count.toLocaleString()} {t("entries")}
                      </span>
                      <Button
                        type="button"
                        variant="outline"
                        onClick={() => void copy(entry.reference)}
                        aria-label={`${t("Copy")} ${entry.reference}`}
                      >
                        {copied === entry.reference ? t("Copied") : t("Copy")}
                      </Button>
                      {onUse && (
                        <Button
                          type="button"
                          variant="outline"
                          onClick={() => onUse(entry, kind)}
                          aria-label={`${t("Create rule")} ${entry.reference}`}
                        >
                          {t("Create rule")}
                        </Button>
                      )}
                    </li>
                  ))}
                </ul>
              )}
              <p className="text-xs text-[var(--muted)]">
                {t(
                  "At most 100 tags are shown. Narrow the search to find more. Xray .dat references are not sing-box rule sets.",
                )}
              </p>
            </>
          )}
        </div>
      )}
    </details>
  );
}
