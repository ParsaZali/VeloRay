import { Send } from "lucide-react";
import type { StoreBrand } from "@/lib/commerce";
import { t } from "@/lib/i18n";
import { Input } from "@/components/ui/controls";
export function StoreTelegram({
  brand,
  editable,
  onChange,
}: {
  brand: StoreBrand;
  editable: boolean;
  onChange: (patch: Partial<StoreBrand>) => void;
}) {
  return (
    <fieldset className="space-y-4 rounded-lg border border-[var(--border)] p-4">
      <legend className="px-2 text-[12px] font-medium">
        {t("Telegram store", "فروشگاه تلگرام")}
      </legend>
      <label className="flex gap-2 text-[12px]">
        <input
          type="checkbox"
          checked={!!brand.telegram_sales}
          disabled={!editable}
          onChange={(e) => onChange({ telegram_sales: e.target.checked })}
        />
        {t(
          "Enable customer bot and Mini App",
          "فعال‌سازی ربات مشتری و مینی‌اپ",
        )}
      </label>
      {brand.telegram_sales && (
        <>
          <label className="label">
            {t("Terms URL", "لینک شرایط خرید")}
            <Input
              type="url"
              dir="ltr"
              required
              value={brand.terms_url || ""}
              disabled={!editable}
              onChange={(e) => onChange({ terms_url: e.target.value })}
            />
          </label>
          <p className="text-[11px] leading-6 text-[var(--muted)]">
            {t(
              "Configure the panel bot in Settings and publish Stars offers. HTTPS, purchase terms and a support link are required.",
              "ربات پنل را در تنظیمات وصل کنید و بسته با واحد استارز بسازید. HTTPS، شرایط خرید و لینک پشتیبانی لازم است.",
            )}
          </p>
          {brand.telegram_url ? (
            <a
              className="inline-flex items-center gap-2 text-[12px] text-emerald-500"
              href={brand.telegram_url}
              target="_blank"
              rel="noreferrer"
            >
              <Send className="h-4 w-4" />
              {t("Open sales bot", "باز کردن ربات فروش")}
            </a>
          ) : (
            <p role="status" className="text-[11px] text-amber-500">
              {t(
                "Test the Telegram bot in Settings to generate its store link.",
                "برای ساخت لینک فروشگاه، ربات تلگرام را در تنظیمات تست کنید.",
              )}
            </p>
          )}
        </>
      )}
    </fieldset>
  );
}
