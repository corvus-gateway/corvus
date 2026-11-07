// New-release notice in the sidebar — Korean source strings + translations.
export const koUpdate = {
  updateBtn: "업데이트",
  updateTitle: "새 버전 {v}이(가) 나왔습니다. 눌러서 릴리스 노트를 확인하세요.",
};

type UpdateKey = keyof typeof koUpdate;

export const updateStrings: Record<"en" | "zh" | "ja" | "ru" | "fr" | "de" | "es", Record<UpdateKey, string>> = {
  en: { updateBtn: "Update", updateTitle: "Version {v} is available. Open the release notes." },
  zh: { updateBtn: "更新", updateTitle: "新版本 {v} 已发布。点击查看发布说明。" },
  ja: { updateBtn: "アップデート", updateTitle: "新しいバージョン {v} が公開されました。クリックしてリリースノートを確認できます。" },
  ru: { updateBtn: "Обновить", updateTitle: "Доступна версия {v}. Откройте описание выпуска." },
  fr: { updateBtn: "Mise à jour", updateTitle: "La version {v} est disponible. Ouvrir les notes de version." },
  de: { updateBtn: "Update", updateTitle: "Version {v} ist verfügbar. Versionshinweise öffnen." },
  es: { updateBtn: "Actualizar", updateTitle: "La versión {v} está disponible. Abrir las notas de la versión." },
};
