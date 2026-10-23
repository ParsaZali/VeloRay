import { useEffect, useRef, useState } from "react";
import { CheckCircle2, Copy, Globe } from "lucide-react";
import { api } from "@/lib/api";
import { t } from "@/lib/i18n";
import type { StoreBrand } from "@/lib/commerce";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/controls";

export function StoreDomain({
  owner,
  brand,
  editable,
  onChange,
  onVerified,
}: {
  owner: number;
  brand: StoreBrand;
  editable: boolean;
  onChange: (patch: Partial<StoreBrand>) => void;
  onVerified: (brand: StoreBrand) => void;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  async function verify() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await api<StoreBrand>(
        `/commerce/store/verify-domain?owner=${owner}`,
        {
          method: "POST",
          body: JSON.stringify({ origin: brand.subscription_origin || "" }),
        },
      );
      if (active.current) onVerified(result);
    } catch (e) {
      if (active.current) setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (active.current) setBusy(false);
    }
  }
  return (
    <fieldset className="space-y-4 rounded-lg border border-[var(--border)] p-4">
      <legend className="px-2 text-[12px] font-medium">
        {t("Subscription branding", "برندینگ صفحهٔ ساب")}
      </legend>
      <label className="flex gap-2 text-[12px]">
        <input
          type="checkbox"
          checked={!!brand.subscription_branding}
          disabled={!editable}
          onChange={(e) =>
            onChange({ subscription_branding: e.target.checked })
          }
        />
        {t(
          "Use this store's name, logo and support on customer subscriptions",
          "استفاده از نام، لوگو و پشتیبانی این فروشگاه در ساب مشتریان",
        )}
      </label>
      {brand.subscription_branding && (
        <>
          <label className="label">
            {t(
              "Custom subscription origin (optional)",
              "آدرس اختصاصی ساب (اختیاری)",
            )}
            <Input
              type="url"
              dir="ltr"
              disabled={!editable}
              placeholder="https://sub.example.com:100"
              value={brand.subscription_origin || ""}
              onChange={(e) =>
                onChange({ subscription_origin: e.target.value })
              }
            />
          </label>
          <p className="text-[11px] leading-6 text-[var(--muted)]">
            {t(
              "Save the domain, add its TXT record and configure HTTPS, then verify to activate its subscription links.",
              "دامنه را ذخیره و رکورد TXT و HTTPS آن را تنظیم کنید؛ سپس برای فعال شدن لینک ساب، تأیید را بزنید.",
            )}
          </p>
          {brand.domain_record && (
            <div className="space-y-3 rounded-lg bg-[var(--surface-2)] p-3">
              <label className="label">
                {t("TXT record name", "نام رکورد TXT")}
                <Input
                  readOnly
                  dir="ltr"
                  value={brand.domain_record.name}
                  onFocus={(e) => e.target.select()}
                />
              </label>
              <label className="label">
                {t("TXT record value", "مقدار رکورد TXT")}
                <Input
                  readOnly
                  dir="ltr"
                  value={brand.domain_record.value}
                  onFocus={(e) => e.target.select()}
                />
              </label>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(
                        brand.domain_record!.value,
                      );
                    } catch {
                      setError(t("Copy failed", "کپی ناموفق بود"));
                    }
                  }}
                >
                  <Copy className="h-4 w-4" />
                  {t("Copy value", "کپی مقدار")}
                </Button>
                {brand.subscription_verified ? (
                  <span className="flex items-center gap-2 text-[12px] text-emerald-500">
                    <CheckCircle2 className="h-4 w-4" />
                    {t("Domain verified", "دامنه تأیید شد")}
                  </span>
                ) : (
                  <Button
                    type="button"
                    disabled={!editable || busy}
                    onClick={() => void verify()}
                  >
                    <Globe className="h-4 w-4" />
                    {t(
                      busy ? "Checking domain…" : "Verify domain",
                      busy ? "در حال بررسی دامنه…" : "تأیید دامنه",
                    )}
                  </Button>
                )}
              </div>
            </div>
          )}
        </>
      )}
      {error && (
        <p role="alert" className="text-[12px] text-red-500">
          {error}
        </p>
      )}
    </fieldset>
  );
}
