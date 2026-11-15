import type { CertReplaceKey } from "../certreplace.i18n";

export const certReplaceStrings: Record<CertReplaceKey, string> = {
  replaceCertAction: "Заменить",
  replaceCertTitle: "Замена сертификата",
  replaceCertDesc: "Вставьте обновлённый сертификат и закрытый ключ. Сертификат сохраняет свой ID и привязки к хостам; хосты, которые его используют, будут развёрнуты заново.",
  certReplaced: "Сертификат заменён",
  renewFailed: "Ошибка продления",
  renewFailedTip: "Продление не удалось; текущий сертификат используется до истечения срока. Ошибка: {e}",
};
