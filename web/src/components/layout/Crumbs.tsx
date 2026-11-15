import { useLocation } from "react-router-dom";
import { useI18n } from "../../i18n";
import { findNav } from "./nav";

/** PatternFly-style breadcrumb: Group / Page. */
export function Crumbs() {
  const { t } = useI18n();
  const cur = findNav(useLocation().pathname);
  if (!cur) return null;
  return (
    <nav aria-label={t("breadcrumb")} className="flex items-center gap-1.5 text-xs text-fg-subtle">
      <span>{t(cur.group)}</span><span aria-hidden>/</span><span className="text-fg-muted">{t(cur.label)}</span>
    </nav>
  );
}
