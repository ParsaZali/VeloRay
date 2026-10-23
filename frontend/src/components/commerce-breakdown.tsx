import { amountLabel, type Reports } from "@/lib/commerce";
import { t } from "@/lib/i18n";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export function CommerceBreakdown({
  reports,
  admin,
}: {
  reports: Reports;
  admin: boolean;
}) {
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>
            {t("Revenue and cost by node", "درآمد و هزینه به تفکیک نود")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {reports.by_node.map((r, index) => (
            <div
              key={`${r.node_id}:${r.currency}:${index}`}
              className="space-y-2 border-b border-[var(--border-subtle)] pb-3 text-[12px]"
            >
              <p className="font-medium">
                {r.node_id
                  ? t("Node", "نود") + ` #${r.node_id}`
                  : t("General costs", "هزینه‌های عمومی")}{" "}
                · {r.currency}
              </p>
              <dl className="grid grid-cols-3 gap-3">
                {[
                  [t("Revenue", "درآمد"), r.revenue || 0],
                  [t("Operating costs", "هزینه‌های عملیاتی"), r.cost],
                  [t("Profit", "سود"), r.profit || 0],
                ].map(([label, value]) => (
                  <div key={label}>
                    <dt className="text-[10px] text-[var(--muted)]">{label}</dt>
                    <dd className="mt-1 tabular-nums">
                      {amountLabel(value, r.currency)}
                    </dd>
                  </div>
                ))}
              </dl>
            </div>
          ))}
          <p className="text-[11px] leading-6 text-[var(--muted)]">
            {t(
              "Revenue is split equally across the offer's nodes at sale time. Recorded operating costs are assigned to their node.",
              "درآمد فروش بین نودهای بسته، در زمان خرید، مساوی تقسیم می‌شود. هزینه‌های ثبت‌شده به نود انتخاب‌شده تعلق دارند.",
            )}
          </p>
        </CardContent>
      </Card>
      {admin && (
        <Card>
          <CardHeader>
            <CardTitle>
              {t("Sales across sellers", "فروش همهٔ فروشندگان")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {reports.by_seller?.map((r) => (
                <div
                  key={`${r.owner_id}:${r.currency}`}
                  className="flex flex-wrap justify-between gap-3 text-[12px]"
                >
                  <span>
                    {r.username} · {r.orders} {t("Orders", "سفارش")}
                  </span>
                  <span className="tabular-nums">
                    {amountLabel(r.revenue, r.currency)}
                  </span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
