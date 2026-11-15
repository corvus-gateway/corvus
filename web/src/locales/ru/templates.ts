import type { K } from "../templates.i18n";

export const templatesStrings: Record<K, string> = {
  fromTemplate: "Добавить из шаблона",
  templatesTitle: "Шаблоны виртуальных патчей",
  templatesDesc: "Правила, блокирующие известные CVE до установки исправлений. Сначала примените их в режиме «Журнал», убедитесь в отсутствии ложных срабатываний, затем переключите на «Блокировать».",
  tplSevCritical: "Критический", tplSevHigh: "Высокий",
  tplProducts: "Затрагивает", tplFalsePositives: "Ложные срабатывания",
  tplExemptList: "Исключённый список IP (сеть администраторов)", tplExemptNone: "Без исключений",
  tplExemptHint: "Этот шаблон может исключить сеть администраторов. Без этого правило применяется ко всем IP.",
  tplApply: "Создать правило", tplCreated: "Правило создано из шаблона", tplSelect: "Выберите шаблон",
  managedRules: "Управляемые правила",
  managedRulesDesc: "Пакет сигнатур проверки тела и шаблонов виртуальных патчей. Применяются только более новые версии с действительной подписью.",
  mrVersion: "Версия", mrSource: "Источник", mrBuiltin: "Встроенный", mrInstalled: "Обновлён", mrSignatures: "Сигнатуры", mrTemplates: "Шаблоны", mrChecked: "Последняя проверка",
  mrUrl: "URL обновлений", mrUrlHint: "Подпись загружается с того же адреса с суффиксом .sig (только https).",
  mrAuto: "Проверять ежедневно", mrCheckNow: "Проверить сейчас",
  mrUpload: "Загрузить вручную", mrUploadHint: "Для установок без интернета загрузите bundle.json и bundle.json.sig.",
  mrBundleFile: "Пакет (bundle.json)", mrSigFile: "Подпись (bundle.json.sig)",
  mrUploaded: "Пакет правил применён", mrUpToDate: "Проверено (новая версия применяется при наличии)", mrSaved: "Настройки обновления сохранены",
};
