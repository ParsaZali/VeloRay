import { useState, type FormEvent } from "react";
import { Pause, Play } from "lucide-react";
import { api } from "@/lib/api";
import { t, useLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/controls";
import { Dialog } from "@/components/common/dialog";

export function CustomerFreeze({
  id,
  name,
  frozen,
  until,
  onChanged,
}: {
  id: number;
  name: string;
  frozen?: boolean;
  until?: string;
  onChanged: () => Promise<void>;
}) {
  useLocale();
  const [open, setOpen] = useState(false),
    [days, setDays] = useState(7),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api(`/customers/${id}/${frozen ? "resume" : "freeze"}`, {
        method: "POST",
        body: JSON.stringify(frozen ? {} : { days }),
      });
      setOpen(false);
      await onChanged();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  const label = frozen
    ? t("Resume subscription", "ادامهٔ اشتراک")
    : t("Freeze subscription", "فریز اشتراک");
  return (
    <>
      <Button
        size="sm"
        variant="outline"
        aria-label={`${label} ${name}`}
        onClick={() => {
          setError("");
          setOpen(true);
        }}
      >
        {frozen ? (
          <Play className="h-3.5 w-3.5" />
        ) : (
          <Pause className="h-3.5 w-3.5" />
        )}
        {frozen ? t("Resume", "ادامه") : t("Freeze", "فریز")}
      </Button>
      {open && (
        <Dialog label={label} onClose={() => !busy && setOpen(false)}>
          <form className="space-y-4" onSubmit={submit}>
            <h3>{label}</h3>
            <p className="text-[13px]">{name}</p>
            {frozen ? (
              <>
                <p className="text-[12px] text-[var(--muted)]">
                  {t(
                    "The saved duration resumes now. Usage and the subscription link are preserved.",
                    "مدت ذخیره‌شده از حالا ادامه پیدا می‌کند. مصرف و لینک ساب حفظ می‌شوند.",
                  )}
                </p>
                {until && (
                  <p className="text-[12px]">
                    {t("Automatic resume", "ادامهٔ خودکار")}:{" "}
                    {new Date(until).toLocaleString()}
                  </p>
                )}
              </>
            ) : (
              <>
                <label className="label">
                  {t("Freeze duration (days)", "مدت فریز (روز)")}
                  <Input
                    className="input"
                    type="number"
                    required
                    min={1}
                    max={30}
                    value={days}
                    onChange={(e) => setDays(Number(e.target.value))}
                  />
                </label>
                <p className="text-[12px] text-[var(--muted)]">
                  {t(
                    "Connections pause and remaining time is saved. The subscription resumes automatically. Total allowance: 30 days per year.",
                    "اتصال‌ها متوقف و زمان باقی‌مانده ذخیره می‌شود. اشتراک خودکار ادامه پیدا می‌کند. سهمیهٔ کل: ۳۰ روز در سال.",
                  )}
                </p>
              </>
            )}
            {error && (
              <p role="alert" className="text-[12px] text-red-500">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setOpen(false)}
              >
                {t("Cancel")}
              </Button>
              <Button type="submit" disabled={busy}>
                {busy ? t("Saving…") : label}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
    </>
  );
}
