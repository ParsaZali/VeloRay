import { useEffect, useState, type FormEvent } from "react";
import { Copy, Link2 } from "lucide-react";
import { api } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { useSession } from "@/lib/session";
import { Button } from "@/components/ui/button";
import { Input, Textarea } from "@/components/ui/controls";
import { Dialog } from "@/components/common/dialog";

type Enrollment = {
  id: string;
  name: string;
  public_host: string;
  expires_at: string;
  used_at?: string;
  revoked_at?: string;
};
export function NodeEnrollment() {
  useLocale();
  const user = useSession();
  const [open, setOpen] = useState(false),
    [name, setName] = useState(""),
    [host, setHost] = useState(""),
    [issued, setIssued] = useState<{
      install_command: string;
      expires_at: string;
    } | null>(null),
    [pending, setPending] = useState<Enrollment[]>([]),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  useEffect(() => {
    if (!open || !user?.is_superuser) return;
    const c = new AbortController();
    void api<Enrollment[]>("/node-enrollment", { signal: c.signal })
      .then(setPending)
      .catch((e) => {
        if (!c.signal.aborted) setError(e.message);
      });
    return () => c.abort();
  }, [open, user?.is_superuser]);
  if (!user?.is_superuser) return null;
  async function issue(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await api<{ install_command: string; expires_at: string }>(
        "/node-enrollment",
        { method: "POST", body: JSON.stringify({ name, public_host: host }) },
      );
      setIssued(result);
      setPending(await api<Enrollment[]>("/node-enrollment"));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <Button
        variant="outline"
        onClick={() => {
          setIssued(null);
          setError("");
          setOpen(true);
        }}
      >
        <Link2 className="h-4 w-4" />
        {t("Connect new node", "اتصال نود جدید")}
      </Button>
      {open && (
        <Dialog
          size="lg"
          label={t("Connect new node", "اتصال نود جدید")}
          onClose={() => !busy && setOpen(false)}
        >
          <div className="space-y-5">
            <h3>{t("Connect new node", "اتصال نود جدید")}</h3>
            {issued ? (
              <>
                <p className="text-[12px] text-[var(--muted)]">
                  {t(
                    "Run this command as root on the new node. It expires in 15 minutes and works once.",
                    "این دستور را با دسترسی روت روی نود جدید اجرا کنید. اعتبار آن ۱۵ دقیقه و یک‌بارمصرف است.",
                  )}
                </p>
                <Textarea
                  readOnly
                  rows={5}
                  dir="ltr"
                  value={issued.install_command}
                  className="input font-mono text-[11px]"
                  onFocus={(e) => e.target.select()}
                />
                <Button
                  variant="outline"
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(
                        issued.install_command,
                      );
                    } catch {
                      setError(
                        t(
                          "Select and copy the command.",
                          "دستور را انتخاب و کپی کنید.",
                        ),
                      );
                    }
                  }}
                >
                  <Copy className="h-4 w-4" />
                  {t("Copy command", "کپی دستور")}
                </Button>
                <p className="text-[11px] text-[var(--muted)]">
                  {t(
                    "The panel needs a trusted HTTPS certificate. The node's own certificate is pinned during registration.",
                    "گواهی HTTPS پنل باید معتبر باشد. گواهی خود نود هنگام ثبت با اثرانگشت تأیید می‌شود.",
                  )}
                </p>
              </>
            ) : (
              <form onSubmit={issue} className="space-y-4">
                <label className="label">
                  {t("Node name")}
                  <Input
                    className="input"
                    required
                    maxLength={96}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                </label>
                <label className="label">
                  {t("Public host")}
                  <Input
                    className="input"
                    required
                    dir="ltr"
                    placeholder="node.example.com"
                    value={host}
                    onChange={(e) => setHost(e.target.value)}
                  />
                </label>
                <Button disabled={busy} type="submit">
                  {t("Create installation command", "ساخت دستور نصب")}
                </Button>
              </form>
            )}
            {error && (
              <p role="alert" className="text-[12px] text-red-500">
                {error}
              </p>
            )}
            {pending.filter(
              (p) =>
                !p.used_at &&
                !p.revoked_at &&
                Date.parse(p.expires_at) > Date.now(),
            ).length > 0 && (
              <div className="space-y-3 border-t border-[var(--border)] pt-4">
                <h4 className="text-[12px] font-semibold">
                  {t("Pending connections", "اتصال‌های در انتظار ثبت")}
                </h4>
                {pending
                  .filter(
                    (p) =>
                      !p.used_at &&
                      !p.revoked_at &&
                      Date.parse(p.expires_at) > Date.now(),
                  )
                  .map((p) => (
                    <div
                      key={p.id}
                      className="flex items-center justify-between gap-3 text-[12px]"
                    >
                      <span>
                        {p.name} · {p.public_host}
                      </span>
                      <Button
                        variant="outline"
                        disabled={busy}
                        onClick={async () => {
                          setBusy(true);
                          try {
                            await api(`/node-enrollment/${p.id}`, {
                              method: "DELETE",
                            });
                            setPending((rows) =>
                              rows.filter((r) => r.id !== p.id),
                            );
                            setIssued(null);
                          } catch (e) {
                            setError(
                              e instanceof Error ? e.message : String(e),
                            );
                          } finally {
                            setBusy(false);
                          }
                        }}
                      >
                        {t("Revoke", "لغو")}
                      </Button>
                    </div>
                  ))}
              </div>
            )}
          </div>
        </Dialog>
      )}
    </>
  );
}
