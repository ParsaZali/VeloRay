import { useState, type ReactNode } from "react";
import { Send, ChevronDown, Eye, EyeOff, Plus, Trash2 } from "lucide-react";
import type { StoreBrand } from "@/lib/commerce";
import {
  defaultSalesBot,
  type SalesBot,
  type Tutorial,
  type BotContent,
} from "@/lib/sales";
import { t } from "@/lib/i18n";
import { Input, Select, Textarea } from "@/components/ui/controls";
import { Button } from "@/components/ui/button";
function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <details className="group rounded-lg border border-[var(--border)]">
      <summary className="flex cursor-pointer list-none items-center justify-between p-4 text-[13px] font-medium">
        {title}
        <ChevronDown className="h-4 w-4 transition-transform group-open:rotate-180" />
      </summary>
      <div className="space-y-4 border-t border-[var(--border)] p-4">
        {children}
      </div>
    </details>
  );
}
export function StoreTelegram({
  brand,
  editable,
  onChange,
  services = [],
}: {
  brand: StoreBrand;
  editable: boolean;
  onChange: (patch: Partial<StoreBrand>) => void;
  services?: { id: number; name: string }[];
}) {
  const bot = { ...defaultSalesBot, ...brand.bot };
  const patch = (value: Partial<SalesBot>) =>
    onChange({ bot: { ...bot, ...value } });
  const [preview, setPreview] = useState(false);
  const textFields: [keyof SalesBot, string, string][] = [
    [
      "welcome",
      t("Welcome message", "پیام خوش‌آمد"),
      t(
        "Welcome to {store}. Choose an option.",
        "به {store} خوش آمدید. از منوی زیر انتخاب کنید.",
      ),
    ],
    ["catalog_text", t("Catalog message", "پیام انتخاب بسته"), ""],
    ["empty_text", t("Empty services message", "پیام نداشتن سرویس"), ""],
    ["delivery_text", t("Delivery message", "پیام تحویل اشتراک"), ""],
    ["trial_text", t("Trial delivery message", "پیام تحویل تست"), ""],
    ["support_text", t("Support message", "متن پشتیبانی"), ""],
    ["tutorial_text", t("Tutorial message", "متن آموزش"), ""],
  ];
  return (
    <fieldset className="min-w-0 space-y-4 rounded-xl border border-[var(--border)] p-4 sm:p-5">
      <legend className="px-2 text-[13px] font-semibold">
        {t("Sales bot & Mini App", "ربات فروش و مینی‌اپ")}
      </legend>
      <label className="flex items-center gap-3 text-[13px]">
        <input
          type="checkbox"
          aria-label={t("Enable sales bot", "روشن کردن ربات فروش")}
          checked={!!brand.telegram_sales}
          disabled={!editable}
          onChange={(e) => onChange({ telegram_sales: e.target.checked })}
        />
        {t("Enable sales bot", "روشن کردن ربات فروش")}
        <span
          className={`ms-auto text-[11px] ${brand.telegram_sales ? "text-emerald-500" : "text-[var(--muted)]"}`}
        >
          {brand.telegram_sales ? t("Enabled", "روشن") : t("Disabled", "خاموش")}
        </span>
      </label>
      <p className="text-[12px] leading-6 text-[var(--muted)]">
        {t(
          "Optional. Turning sales off keeps the owner management bot available.",
          "اختیاری است. با خاموش کردن فروش، ربات مدیریت مدیران همچنان فعال می‌ماند.",
        )}
      </p>
      {brand.telegram_sales && (
        <>
          <label className="label">
            {t("Purchase terms URL", "لینک شرایط خرید")}
            <Input
              type="url"
              dir="ltr"
              required
              value={brand.terms_url || ""}
              disabled={!editable}
              onChange={(e) => onChange({ terms_url: e.target.value })}
            />
          </label>
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="label">
              {t("Customer language", "زبان مشتری")}
              <Select
                value={bot.locale}
                disabled={!editable}
                onChange={(e) => patch({ locale: e.target.value })}
              >
                <option value="fa">فارسی</option>
                <option value="en">English</option>
              </Select>
            </label>
            <label className="label">
              {t("Menu style", "نوع منو")}
              <Select
                value={bot.menu_mode}
                disabled={!editable}
                onChange={(e) => patch({ menu_mode: e.target.value })}
              >
                <option value="inline">
                  {t("Inline buttons", "دکمه‌های شیشه‌ای")}
                </option>
                <option value="keyboard">
                  {t("Persistent keyboard", "کیبورد پایین چت")}
                </option>
              </Select>
            </label>
          </div>
          <Section title={t("Messages & images", "متن‌ها و تصاویر")}>
            <p className="text-[11px] text-[var(--muted)]">
              {t(
                "Leave blank for defaults. Variables: {store}, {user_id}.",
                "خالی = متن پیش‌فرض. متغیرها: {store} و {user_id}.",
              )}
            </p>
            {textFields.map(([key, label, placeholder]) => (
              <label key={key} className="label">
                {label}
                <Textarea
                  rows={3}
                  maxLength={900}
                  placeholder={placeholder}
                  disabled={!editable}
                  value={String(bot[key])}
                  onChange={(e) => patch({ [key]: e.target.value })}
                />
              </label>
            ))}
            {(
              ["welcome_image", "catalog_image", "tutorial_image"] as const
            ).map((key, i) => (
              <label key={key} className="label">
                {
                  [
                    t("Welcome image URL", "نشانی تصویر خوش‌آمد"),
                    t("Catalog image URL", "نشانی تصویر فروشگاه"),
                    t("Tutorial image URL", "نشانی تصویر آموزش"),
                  ][i]
                }
                <Input
                  type="url"
                  dir="ltr"
                  placeholder="https://"
                  disabled={!editable}
                  value={bot[key]}
                  onChange={(e) => patch({ [key]: e.target.value })}
                />
              </label>
            ))}
          </Section>
          <Section title={t("Main menu", "منوی اصلی")}>
            <p className="text-[11px] text-[var(--muted)]">
              {t(
                "Edit labels, visibility and row number. Buttons on the same row appear together.",
                "نام، نمایش و شماره ردیف هر دکمه را تغییر دهید. دکمه‌های هم‌ردیف کنار هم قرار می‌گیرند.",
              )}
            </p>
            {bot.buttons.map((button, i) => (
              <div
                key={button.action}
                className="flex flex-wrap items-end gap-2"
              >
                <label className="label min-w-40 flex-1">
                  <span className="font-mono text-[10px] text-[var(--muted)]">
                    {button.action}
                  </span>
                  <Input
                    aria-label={`${button.action} label`}
                    maxLength={48}
                    value={button.label}
                    disabled={!editable}
                    onChange={(e) =>
                      patch({
                        buttons: bot.buttons.map((b, j) =>
                          i === j ? { ...b, label: e.target.value } : b,
                        ),
                      })
                    }
                  />
                </label>
                <label className="label w-20">
                  {t("Row", "ردیف")}
                  <Input
                    type="number"
                    min={0}
                    max={11}
                    value={button.row}
                    disabled={!editable}
                    onChange={(e) =>
                      patch({
                        buttons: bot.buttons.map((b, j) =>
                          i === j ? { ...b, row: Number(e.target.value) } : b,
                        ),
                      })
                    }
                  />
                </label>
                <Button
                  type="button"
                  variant="ghost"
                  disabled={!editable}
                  aria-label={t("Toggle visibility", "تغییر نمایش")}
                  aria-pressed={button.visible}
                  onClick={() =>
                    patch({
                      buttons: bot.buttons.map((b, j) =>
                        i === j ? { ...b, visible: !b.visible } : b,
                      ),
                    })
                  }
                >
                  {button.visible ? (
                    <Eye className="h-4 w-4" />
                  ) : (
                    <EyeOff className="h-4 w-4" />
                  )}
                </Button>
              </div>
            ))}
          </Section>
          <Section title={t("Free trial", "اشتراک آزمایشی")}>
            <label className="flex gap-2 text-[12px]">
              <input
                type="checkbox"
                disabled={!editable}
                checked={bot.trial_enabled}
                onChange={(e) => patch({ trial_enabled: e.target.checked })}
              />
              {t("Offer a free trial", "فعال‌سازی تست رایگان")}
            </label>
            {bot.trial_enabled && (
              <>
                <div className="grid gap-3 sm:grid-cols-2">
                  <label className="label">
                    {t("Trial service", "سرویس تست")}
                    <Select
                      required
                      value={bot.trial_service_id}
                      disabled={!editable}
                      onChange={(e) =>
                        patch({ trial_service_id: Number(e.target.value) })
                      }
                    >
                      <option value={0}>
                        {t("Choose a service", "انتخاب سرویس")}
                      </option>
                      {services.map((s) => (
                        <option key={s.id} value={s.id}>
                          {s.name}
                        </option>
                      ))}
                    </Select>
                  </label>
                  <label className="label">
                    {t("Eligibility", "شرایط دریافت")}
                    <Select
                      value={bot.trial_mode}
                      disabled={!editable}
                      onChange={(e) => patch({ trial_mode: e.target.value })}
                    >
                      <option value="once">
                        {t(
                          "Once per Telegram user",
                          "یک‌بار برای هر کاربر تلگرام",
                        )}
                      </option>
                      <option value="paid_only">
                        {t(
                          "Once · existing customers only",
                          "یک‌بار · فقط مشتریان دارای خرید",
                        )}
                      </option>
                    </Select>
                  </label>
                  <label className="label">
                    {t("Traffic (GiB)", "حجم (گیگابایت)")}
                    <Input
                      type="number"
                      min={0.001}
                      max={100}
                      step={0.001}
                      value={bot.trial_traffic_bytes / 1073741824}
                      disabled={!editable}
                      onChange={(e) =>
                        patch({
                          trial_traffic_bytes: Math.round(
                            Number(e.target.value) * 1073741824,
                          ),
                        })
                      }
                    />
                  </label>
                  <label className="label">
                    {t("Duration (hours)", "مدت (ساعت)")}
                    <Input
                      type="number"
                      min={1}
                      max={168}
                      value={bot.trial_hours}
                      disabled={!editable}
                      onChange={(e) =>
                        patch({ trial_hours: Number(e.target.value) })
                      }
                    />
                  </label>
                  <label className="label">
                    {t(
                      "Required channel · optional",
                      "کانال عضویت اجباری · اختیاری",
                    )}
                    <Input
                      dir="ltr"
                      placeholder="@channel"
                      value={bot.trial_channel}
                      disabled={!editable}
                      onChange={(e) => patch({ trial_channel: e.target.value })}
                    />
                  </label>
                  <label className="label">
                    {t("Channel join URL", "لینک عضویت کانال")}
                    <Input
                      type="url"
                      dir="ltr"
                      placeholder="https://t.me/…"
                      value={bot.trial_channel_url}
                      disabled={!editable}
                      onChange={(e) =>
                        patch({ trial_channel_url: e.target.value })
                      }
                    />
                  </label>
                </div>
                <p className="text-[11px] text-[var(--muted)]">
                  {t(
                    "For membership checks, add the bot as a channel administrator.",
                    "برای بررسی عضویت، ربات را مدیر کانال کنید.",
                  )}
                </p>
              </>
            )}
          </Section>
          <Section
            title={t(
              "Tutorials & custom buttons",
              "آموزش‌ها و دکمه‌های دلخواه",
            )}
          >
            <ContentEditor<Tutorial>
              items={bot.tutorials}
              labelKey="title"
              editable={editable}
              onChange={(tutorials) => patch({ tutorials })}
              title={t("Tutorial", "آموزش")}
            />
            <ContentEditor<BotContent>
              items={bot.custom_buttons}
              labelKey="label"
              editable={editable}
              onChange={(custom_buttons) => patch({ custom_buttons })}
              title={t("Custom button", "دکمه دلخواه")}
            />
          </Section>
          <Section title={t("Customer permissions", "دسترسی مشتری")}>
            <label className="flex gap-2 text-[12px]">
              <input
                type="checkbox"
                disabled={!editable}
                checked={bot.allow_toggle}
                onChange={(e) => patch({ allow_toggle: e.target.checked })}
              />
              {t(
                "Allow enabling/disabling own subscription",
                "اجازه روشن و خاموش کردن سرویس خود",
              )}
            </label>
            <label className="flex gap-2 text-[12px]">
              <input
                type="checkbox"
                disabled={!editable}
                checked={bot.allow_rotate}
                onChange={(e) => patch({ allow_rotate: e.target.checked })}
              />
              {t(
                "Allow changing own subscription link",
                "اجازه تغییر لینک اشتراک خود",
              )}
            </label>
          </Section>
          <Button
            type="button"
            variant="outline"
            onClick={() => setPreview(!preview)}
          >
            {t("Menu preview", "پیش‌نمایش منو")}
          </Button>
          {preview && (
            <div
              dir={bot.locale === "fa" ? "rtl" : "ltr"}
              className="space-y-3 rounded-xl border border-[var(--border)] bg-[var(--bg)] p-4"
            >
              {bot.welcome_image && (
                <img
                  alt=""
                  src={bot.welcome_image}
                  className="max-h-44 rounded-lg object-contain"
                />
              )}
              <p className="whitespace-pre-wrap text-[13px] leading-7">
                {(
                  bot.welcome ||
                  t("Welcome to {store}.", "به {store} خوش آمدید.")
                )
                  .replaceAll("{store}", brand.name)
                  .replaceAll("{user_id}", "123456789")}
              </p>
              {[
                ...new Set(
                  bot.buttons
                    .filter(
                      (b) =>
                        b.visible &&
                        (b.action !== "trial" || bot.trial_enabled),
                    )
                    .map((b) => b.row),
                ),
              ]
                .sort((a, b) => a - b)
                .map((row) => (
                  <div key={row} className="flex flex-wrap gap-2">
                    {bot.buttons
                      .filter(
                        (b) =>
                          b.row === row &&
                          b.visible &&
                          (b.action !== "trial" || bot.trial_enabled),
                      )
                      .map((b) => (
                        <span
                          key={b.action}
                          className="flex-1 rounded-lg border border-emerald-500/20 bg-emerald-500/10 p-3 text-center text-[12px]"
                        >
                          {b.label}
                        </span>
                      ))}
                  </div>
                ))}
            </div>
          )}
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
                "Check the bot connection in Settings to generate its store link.",
                "برای ساخت لینک فروشگاه، اتصال ربات را در تنظیمات بررسی کنید.",
              )}
            </p>
          )}
        </>
      )}
    </fieldset>
  );
}
function ContentEditor<T extends Tutorial | BotContent>({
  items,
  labelKey,
  editable,
  onChange,
  title,
}: {
  items: T[];
  labelKey: "title" | "label";
  editable: boolean;
  onChange: (items: T[]) => void;
  title: string;
}) {
  const update = (i: number, key: string, value: string) =>
    onChange(items.map((d, j) => (j === i ? { ...d, [key]: value } : d)));
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-[12px] font-medium">{title}</h4>
        <Button
          type="button"
          variant="ghost"
          disabled={!editable || items.length >= 12}
          onClick={() =>
            onChange([
              ...items,
              { [labelKey]: "", url: "", text: "", image: "" } as T,
            ])
          }
        >
          <Plus className="h-4 w-4" />
          {t("Add", "افزودن")}
        </Button>
      </div>
      {items.map((item, i) => (
        <div
          key={i}
          className="space-y-3 rounded-lg border border-[var(--border)] p-3"
        >
          <div className="flex gap-2">
            <Input
              aria-label={title}
              placeholder={title}
              required
              maxLength={labelKey === "title" ? 80 : 48}
              disabled={!editable}
              value={
                labelKey === "title"
                  ? (item as Tutorial).title
                  : (item as BotContent).label
              }
              onChange={(e) => update(i, labelKey, e.target.value)}
            />
            <Button
              type="button"
              variant="ghost"
              aria-label={t("Delete", "حذف")}
              disabled={!editable}
              onClick={() => onChange(items.filter((_, j) => i !== j))}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
          <Textarea
            aria-label={t("Text", "متن")}
            placeholder={t("Message text", "متن پیام")}
            maxLength={900}
            disabled={!editable}
            value={item.text}
            onChange={(e) => update(i, "text", e.target.value)}
          />
          <Input
            aria-label={t("Link URL", "نشانی لینک")}
            type="url"
            dir="ltr"
            placeholder="https://…"
            disabled={!editable}
            value={item.url}
            onChange={(e) => update(i, "url", e.target.value)}
          />
          <Input
            aria-label={t("Image URL", "نشانی تصویر")}
            type="url"
            dir="ltr"
            placeholder={t("Image URL · optional", "نشانی تصویر · اختیاری")}
            disabled={!editable}
            value={item.image}
            onChange={(e) => update(i, "image", e.target.value)}
          />
        </div>
      ))}
    </div>
  );
}
export function SalesPayments({
  brand,
  editable,
  onChange,
}: {
  brand: StoreBrand;
  editable: boolean;
  onChange: (patch: Partial<StoreBrand>) => void;
}) {
  const bot = { ...defaultSalesBot, ...brand.bot };
  const patch = (p: Partial<SalesBot>) => onChange({ bot: { ...bot, ...p } });
  return (
    <fieldset className="space-y-4 rounded-xl border border-[var(--border)] p-4">
      <legend className="px-2 text-[13px] font-semibold">
        {t("Payment methods", "روش‌های پرداخت")}
      </legend>
      <p className="text-[12px] leading-6 text-[var(--muted)]">
        {t(
          "Stars invoices are automatic. Bank transfers and USDT require receipt review and administrator approval in this panel or bot.",
          "فاکتور استارز خودکار است. کارت‌به‌کارت و تتر پس از ثبت رسید و تأیید مدیر در همین پنل یا ربات تحویل می‌شوند.",
        )}
      </p>
      <label className="flex gap-2 text-[12px]">
        <input
          type="checkbox"
          checked={bot.card_enabled}
          disabled={!editable}
          onChange={(e) => patch({ card_enabled: e.target.checked })}
        />
        {t("Bank card transfer", "کارت‌به‌کارت")}
      </label>
      {bot.card_enabled && (
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="label">
            {t("Card number", "شماره کارت")}
            <Input
              dir="ltr"
              inputMode="numeric"
              pattern="[0-9]{16}"
              required
              maxLength={16}
              value={bot.card_number}
              disabled={!editable}
              onChange={(e) =>
                patch({ card_number: e.target.value.replace(/\s/g, "") })
              }
            />
          </label>
          <label className="label">
            {t("Cardholder", "صاحب کارت")}
            <Input
              required
              maxLength={160}
              value={bot.card_holder}
              disabled={!editable}
              onChange={(e) => patch({ card_holder: e.target.value })}
            />
          </label>
          <label className="label sm:col-span-2">
            {t("Transfer instructions", "راهنمای واریز")}
            <Textarea
              maxLength={1200}
              value={bot.card_instructions}
              disabled={!editable}
              onChange={(e) => patch({ card_instructions: e.target.value })}
            />
          </label>
        </div>
      )}
      <label className="flex gap-2 text-[12px]">
        <input
          type="checkbox"
          checked={bot.usdt_enabled}
          disabled={!editable}
          onChange={(e) => patch({ usdt_enabled: e.target.checked })}
        />
        {t("Tether (USDT)", "تتر (USDT)")}
      </label>
      {bot.usdt_enabled && (
        <div className="space-y-3">
          <label className="label">
            {t("Network", "شبکه")}
            <Select
              value={bot.usdt_network}
              disabled={!editable}
              onChange={(e) => patch({ usdt_network: e.target.value })}
            >
              {["TRC20", "ERC20", "BEP20"].map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </Select>
          </label>
          <label className="label">
            {t("Receiving wallet", "آدرس کیف پول دریافت")}
            <Input
              dir="ltr"
              required
              value={bot.usdt_address}
              disabled={!editable}
              onChange={(e) => patch({ usdt_address: e.target.value.trim() })}
            />
          </label>
          <label className="label">
            {t("Transfer instructions", "راهنمای انتقال")}
            <Textarea
              maxLength={1200}
              value={bot.usdt_instructions}
              disabled={!editable}
              onChange={(e) => patch({ usdt_instructions: e.target.value })}
            />
          </label>
        </div>
      )}
    </fieldset>
  );
}
