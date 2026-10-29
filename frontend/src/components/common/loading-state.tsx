import { t } from "@/lib/i18n";

export function LoadingState({ rows = 4 }: { rows?: number }) {
  return (
    <div className="workspace-loading" role="status" aria-busy="true">
      <span className="sr-only">{t("Loading…")}</span>
      <div aria-hidden="true">
        <div className="loading-toolbar">
          <i />
          <i />
        </div>
        {Array.from({ length: rows }, (_, index) => (
          <div className="loading-row" key={index}>
            <i />
            <span>
              <i />
              <i />
            </span>
            <i />
          </div>
        ))}
      </div>
    </div>
  );
}
