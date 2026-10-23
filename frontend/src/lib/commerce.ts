import { localeName, t } from "./i18n";
export type Offer = {
  id: number;
  owner_id: number;
  name: string;
  description: string;
  enabled: boolean;
  service_id: number;
  amount: number;
  currency: string;
  duration_days: number;
  traffic_bytes: number;
  cost_amount: number;
};
export type Order = {
  id: string;
  token?: string;
  owner_id: number;
  customer_id?: number;
  payer_id?: number;
  offer_id?: number;
  amount: number;
  currency: string;
  state: string;
  kind: string;
  gateway: string;
  payment_reference?: string;
  created_at: string;
  paid_at?: string;
  snapshot: {
    name?: string;
    customer_name?: string;
    duration_days?: number;
    traffic_bytes?: number;
  };
  last_error: string;
  subscription_url?: string;
  test_mode?: boolean;
};
export type StoreBrand = {
  telegram_sales?: boolean;
  telegram_url?: string;
  terms_url?: string;
  subscription_branding?: boolean;
  subscription_origin?: string;
  subscription_verified?: boolean;
  domain_record?: { name: string; value: string; verified: boolean };
  enabled: boolean;
  name: string;
  description: string;
  logo_url: string;
  support_url: string;
  url?: string;
  owner_id?: number;
};
export type Catalog = StoreBrand & {
  telegram_checkout?: boolean;
  offers: Offer[];
  payments_enabled: boolean;
};
export type Wallet = {
  currency: string;
  balance: number;
  credit_limit: number;
};
export type LedgerEntry = {
  id: number;
  currency: string;
  amount: number;
  balance_after: number;
  reference: string;
  note: string;
  created_at: string;
};
export type Gateway = {
  payments_enabled: boolean;
  merchant_configured: boolean;
  sandbox: boolean;
};
export type Reports = {
  summary: {
    currency: string;
    revenue: number | string;
    orders: number;
    unit_cost: number | string;
    operating_cost: number | string;
    profit: number | string;
  }[];
  by_offer: {
    offer_id: number;
    currency: string;
    revenue: number | string;
    orders: number;
  }[];
  by_node: {
    node_id: number | null;
    currency: string;
    cost: number | string;
    revenue?: number | string;
    profit?: number | string;
  }[];
  by_seller?: {
    owner_id: number;
    username: string;
    currency: string;
    revenue: number | string;
    orders: number;
  }[];
};
export const commerceError = (e: unknown) =>
  e instanceof Error ? e.message : String(e);
export function amountLabel(amount: number | string, currency: string) {
  let value = "—";
  try {
    const integer = BigInt(amount);
    const formatter = new Intl.NumberFormat(localeName(), {
      maximumFractionDigits: 0,
    });
    if (["USD", "EUR"].includes(currency)) {
      const negative = integer < 0n,
        absolute = negative ? -integer : integer,
        whole = absolute / 100n,
        remainder = absolute % 100n;
      value = formatter.format(
        negative && whole === 0n ? -0 : negative ? -whole : whole,
      );
      if (remainder) {
        const separator =
          new Intl.NumberFormat(localeName())
            .formatToParts(1.1)
            .find((p) => p.type === "decimal")?.value || ".";
        value +=
          separator +
          new Intl.NumberFormat(localeName(), {
            minimumIntegerDigits: 2,
            useGrouping: false,
          }).format(remainder);
      }
    } else value = formatter.format(integer);
  } catch {
    /* A malformed amount is never silently presented as zero. */
  }
  const unit =
    currency === "IRT"
      ? t("toman", "تومان")
      : currency === "IRR"
        ? t("rial", "ریال")
        : currency === "XTR"
          ? t("Stars", "استارز")
          : currency;
  return `${value} ${unit}`;
}
export const tokenFromURL = (raw: string) => {
  try {
    return new URL(raw).pathname.split("/").filter(Boolean).at(-1) || "";
  } catch {
    return raw.trim();
  }
};
export function rememberOrder(order: { id: string; token?: string }) {
  if (order.token)
    sessionStorage.setItem(`veloray-order:${order.id}`, order.token);
}

export function orderStatus(status: string) {
  return (
    {
      pending: t("Awaiting payment", "در انتظار پرداخت"),
      paid: t("Paid · delivery pending", "پرداخت‌شده · در انتظار تحویل"),
      fulfilled: t("Delivered", "تحویل‌شده"),
      cancelled: t("Cancelled", "لغوشده"),
    }[status] || status
  );
}
