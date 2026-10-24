import { Select, Input } from "@/components/ui/controls";
import { useLocale, t } from "@/lib/i18n";
import { useEffect, useState, type FormEvent } from "react";
import {
  CheckCircle2,
  Database,
  FileUp,
  Loader2,
  ShieldCheck,
} from "lucide-react";
import {
  api,
  type NodeItem,
  type ImportPreview,
  type ImportResult,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/common/page-header";

const bytes = (n: number) => `${(n / 1024 ** 3).toFixed(3)} ${t("GB")}`;

export function ImportsPage({ embedded = false }: { embedded?: boolean }) {
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [node, setNode] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [ports, setPorts] = useState<Record<string, string>>({});
  const [result, setResult] = useState<ImportResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [acknowledged, setAcknowledged] = useState(false);
  const { locale } = useLocale();
  const fa = locale === "fa";
  useEffect(() => {
    let current = true;
    api<NodeItem[]>("/nodes/")
      .then((items) => {
        if (current) {
          setNodes(items);
          setNode(
            String(items.find((n) => n.is_local)?.id || items[0]?.id || ""),
          );
        }
      })
      .catch((e) => {
        if (current) setMessage(e.message);
      });
    return () => {
      current = false;
    };
  }, []);
  function clearPreview() {
    setPreview(null);
    setResult(null);
    setAcknowledged(false);
    setMessage("");
  }
  async function inspect(e: FormEvent) {
    e.preventDefault();
    if (!file || !node) return;
    if (file.size > 64 * 1024 * 1024) {
      setMessage(
        t("Maximum backup size is 64 MiB.", "حداکثر حجم فایل ۶۴ مگابایت است."),
      );
      return;
    }
    setBusy(true);
    clearPreview();
    try {
      const data = new FormData();
      data.append("node", node);
      data.append("file", file);
      const next = await api<ImportPreview>("/imports/preview", {
        method: "POST",
        body: data,
      });
      setPreview(next);
      setSelected(
        new Set(
          next.inbounds
            .filter((i) => i.supported && !i.existing_id)
            .map((i) => i.source_id),
        ),
      );
      setPorts({});
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "Import preview failed");
    } finally {
      setBusy(false);
    }
  }
  async function commit() {
    if (!preview || !acknowledged || !selected.size) return;
    setBusy(true);
    setMessage("");
    try {
      const overrides: Record<string, number> = {};
      for (const [id, value] of Object.entries(ports))
        if (selected.has(id) && value.trim()) {
          const number = Number(value);
          if (!Number.isInteger(number) || number < 1 || number > 65535)
            throw new Error(
              t(
                "Ports must be integers between 1 and 65535.",
                "پورت باید عددی بین ۱ و ۶۵۵۳۵ باشد.",
              ),
            );
          overrides[id] = number;
        }
      const created = await api<ImportResult>("/imports/commit", {
        method: "POST",
        body: JSON.stringify({
          token: preview.token,
          selected: [...selected],
          port_overrides: overrides,
        }),
      });
      setResult(created);
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "Import failed");
    } finally {
      setBusy(false);
    }
  }
  function select(id: string, checked: boolean) {
    setSelected((previous) => {
      const next = new Set(previous);
      checked ? next.add(id) : next.delete(id);
      return next;
    });
    setAcknowledged(false);
  }
  return (
    <div className="space-y-5" dir={fa ? "rtl" : "ltr"}>
      {!embedded && (
        <PageHeader
          title={t("Import data", "واردسازی داده‌ها")}
          description={t(
            "Migrate clients, shared quotas and inbounds with a review before importing.",
            "انتقال کلاینت‌ها، مصرف و سهمیه‌های مشترک و اینباندها با پیش‌نمایش.",
          )}
        />
      )}
      <div className="grid gap-3 sm:grid-cols-3">
        {[
          t("1 · Select backup", "۱ · انتخاب فایل"),
          t("2 · Review data", "۲ · بررسی اطلاعات"),
          t("3 · Import", "۳ · واردسازی"),
        ].map((step, index) => (
          <div
            key={index}
            className={`rounded-lg border px-4 py-3 text-[12px] ${index === (result ? 2 : preview ? 1 : 0) ? "border-[var(--brand)] bg-[var(--brand-soft)]" : "border-[var(--border)] bg-[var(--card)] text-[var(--muted)]"}`}
          >
            {step}
          </div>
        ))}
      </div>
      {message && (
        <div
          role="alert"
          className="rounded-lg border border-red-500/25 bg-red-500/10 px-4 py-3 text-[12px] text-red-500"
        >
          {message}
        </div>
      )}
      {result ? (
        <Card>
          <CardContent className="space-y-4 p-6">
            <CheckCircle2 className="h-8 w-8 text-emerald-500" />
            <h3 className="text-lg font-semibold">
              {t("Data imported", "اطلاعات وارد شد")}
            </h3>
            <p className="text-[13px] text-[var(--muted)]">
              {result.inbounds_created} {t("inbounds", "اینباند")} ·{" "}
              {result.client_links_created}{" "}
              {t("client connections", "اتصال کلاینت")} ·{" "}
              {result.shared_accounts} {t("shared accounts", "حساب مشترک")}
            </p>
            <p className="rounded-lg bg-[var(--surface-2)] p-4 text-[12px] leading-6">
              {t(
                "Imported inbounds are disabled. Review their ports, TLS files and node settings in Inbounds, then enable them. Existing services keep running.",
                "اینباندهای واردشده غیرفعال هستند. پورت‌ها، فایل‌های TLS و تنظیمات نود را در بخش Inbounds بررسی کنید و سپس آن‌ها را فعال کنید. سرویس‌های فعلی به کار خود ادامه می‌دهند.",
              )}
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                onClick={() => {
                  history.pushState({}, "", "/inbounds");
                  window.dispatchEvent(new PopStateEvent("popstate"));
                }}
              >
                {t("Review inbounds", "بررسی اینباندها")}
              </Button>
              <Button variant="outline" onClick={clearPreview}>
                {t("Import another backup", "انتخاب فایل دیگر")}
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : (
        <>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Database className="h-4 w-4" />
                {t("Backup source", "فایل مبدا")}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <form onSubmit={inspect} className="grid gap-4 sm:grid-cols-2">
                <label className="label">
                  {t("Target node", "نود مقصد")}
                  <Select
                    className="input"
                    value={node}
                    disabled={busy}
                    onChange={(e) => {
                      setNode(e.target.value);
                      clearPreview();
                    }}
                    required
                  >
                    <option value="">{t("Select a node", "انتخاب نود")}</option>
                    {nodes.map((n) => (
                      <option key={n.id} value={n.id}>
                        {n.name} · {n.public_host}
                      </option>
                    ))}
                  </Select>
                </label>
                <label className="label">
                  {t("Backup file", "فایل بکاپ")}
                  <span className="backup-file-field">
                    <span className="backup-file-button">
                      {t("Choose file", "انتخاب فایل")}
                    </span>
                    <span className="min-w-0 flex-1 truncate px-3 text-start">
                      {file?.name ||
                        t("No file selected", "فایلی انتخاب نشده است")}
                    </span>
                    <Input
                      className="backup-file-input"
                      aria-label={t("Backup file", "فایل بکاپ")}
                      type="file"
                      accept=".db,.sqlite,.sqlite3,.json"
                      disabled={busy}
                      onChange={(e) => {
                        setFile(e.target.files?.[0] || null);
                        clearPreview();
                      }}
                      required
                    />
                  </span>
                </label>
                <div className="sm:col-span-2 flex flex-wrap items-center justify-between gap-3">
                  <span className="text-[11px] text-[var(--muted)]">
                    {file
                      ? `${file.name} · ${(file.size / 1024 ** 2).toFixed(2)} MiB`
                      : t("No backup selected", "فایلی انتخاب نشده است")}
                  </span>
                  <Button type="submit" disabled={busy || !file || !node}>
                    {busy ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <FileUp className="h-4 w-4" />
                    )}
                    {t("Preview backup", "پیش‌نمایش بکاپ")}
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
          {preview && (
            <>
              <div className="grid gap-3 sm:grid-cols-3">
                {[
                  [t("Accounts", "حساب‌ها"), String(preview.accounts)],
                  [
                    t("Client connections", "اتصال‌های کلاینت"),
                    String(preview.client_links),
                  ],
                  [
                    t("Preserved usage", "مصرف حفظ‌شده"),
                    bytes(preview.used_traffic_bytes),
                  ],
                ].map(([label, value]) => (
                  <Card key={label}>
                    <CardContent className="p-4">
                      <p className="text-[11px] text-[var(--muted)]">{label}</p>
                      <p className="mt-2 text-2xl font-semibold">{value}</p>
                    </CardContent>
                  </Card>
                ))}
              </div>
              <Card>
                <CardHeader>
                  <CardTitle>
                    {t("Review inbounds", "بررسی اینباندها")}
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-3 p-4">
                  {preview.inbounds.map((i) => (
                    <div
                      key={i.source_id}
                      className={`rounded-lg border p-4 ${selected.has(i.source_id) ? "border-[var(--brand)]/40" : "border-[var(--border)]"}`}
                    >
                      <div className="flex flex-wrap items-center justify-between gap-3">
                        <label className="flex min-w-0 items-start gap-3">
                          <Input
                            type="checkbox"
                            className="mt-1"
                            checked={selected.has(i.source_id)}
                            disabled={
                              busy || !i.supported || Boolean(i.existing_id)
                            }
                            onChange={(e) =>
                              select(i.source_id, e.target.checked)
                            }
                            aria-label={`${t("Import", "واردسازی")} ${i.name}`}
                          />
                          <span>
                            <span className="block break-words text-[13px] font-semibold">
                              {i.name}
                            </span>
                            <span className="mt-1 block text-[11px] text-[var(--muted)]">
                              {i.protocol.toUpperCase()} · {i.transport} ·{" "}
                              {i.security} · {i.client_count}{" "}
                              {t("clients", "کلاینت")}
                            </span>
                          </span>
                        </label>
                        <div className="flex items-center gap-2">
                          <Badge
                            tone={
                              !i.supported ||
                              i.conflict_id ||
                              i.port_state === "conflict"
                                ? "amber"
                                : "green"
                            }
                          >
                            {i.existing_id
                              ? t("Already imported", "قبلاً وارد شده")
                              : !i.supported
                                ? t("Unsupported", "ناسازگار")
                                : i.conflict_id || i.port_state === "conflict"
                                  ? t("Port conflict", "تداخل پورت")
                                  : t("Ready to stage", "آمادهٔ ذخیره")}
                          </Badge>
                          <label className="label w-24">
                            {t("Port", "پورت")}
                            <Input
                              aria-label={`${t("Port for", "پورت")} ${i.name}`}
                              className="input h-8"
                              type="number"
                              min="1"
                              max="65535"
                              value={ports[i.source_id] ?? String(i.port)}
                              disabled={busy || !selected.has(i.source_id)}
                              onChange={(e) => {
                                setPorts((previous) => ({
                                  ...previous,
                                  [i.source_id]: e.target.value,
                                }));
                                setAcknowledged(false);
                              }}
                            />
                          </label>
                        </div>
                      </div>
                      {(i.warnings.length > 0 || i.port_owner) && (
                        <ul className="mt-3 space-y-1 border-t border-[var(--border)] pt-3 text-[11px] leading-5 text-[var(--muted)]">
                          {i.warnings.map((warning, index) => (
                            <li key={index}>{t(warning)}</li>
                          ))}
                          {i.port_owner && (
                            <li>
                              {t("Port owner", "پردازش صاحب پورت")}:{" "}
                              {i.port_owner}
                            </li>
                          )}
                        </ul>
                      )}
                    </div>
                  ))}
                </CardContent>
              </Card>
              <Card>
                <CardContent className="space-y-4 p-4">
                  <div className="flex items-start gap-3">
                    <ShieldCheck className="mt-1 h-5 w-5 shrink-0 text-amber-500" />
                    <div className="space-y-2 text-[12px] leading-6">
                      <p className="font-medium">
                        {t("Migration review", "بررسی مهاجرت")}
                      </p>
                      <ul className="space-y-1 text-[var(--muted)]">
                        {preview.warnings.map((warning, index) => (
                          <li key={index}>{t(warning)}</li>
                        ))}
                      </ul>
                    </div>
                  </div>
                  <label className="flex items-start gap-3 rounded-md bg-[var(--surface-2)] p-3 text-[12px] leading-6">
                    <Input
                      type="checkbox"
                      className="mt-1.5"
                      checked={acknowledged}
                      disabled={busy}
                      onChange={(e) => setAcknowledged(e.target.checked)}
                    />
                    {t(
                      "I reviewed the warnings. Save selected inbounds disabled so I can verify them before activation.",
                      "هشدارها را بررسی کردم. موارد انتخاب‌شده غیرفعال ذخیره شوند تا قبل از فعال‌سازی بررسی‌شان کنم.",
                    )}
                  </label>
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <span className="text-[11px] text-[var(--muted)]">
                      {selected.size}{" "}
                      {t(
                        "inbounds selected · preview valid for 30 minutes",
                        "اینباند انتخاب‌شده · اعتبار پیش‌نمایش ۳۰ دقیقه",
                      )}
                    </span>
                    <Button
                      onClick={commit}
                      disabled={busy || !selected.size || !acknowledged}
                    >
                      {busy && <Loader2 className="h-4 w-4 animate-spin" />}
                      {t("Import selected data", "واردسازی موارد انتخاب‌شده")}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            </>
          )}
        </>
      )}
    </div>
  );
}
