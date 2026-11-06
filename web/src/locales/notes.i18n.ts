// Notes on proxy hosts, redirection hosts and streams — Korean source strings + translations.
export const koNotes = {
  secNotes: "메모",
  notesHint: "이 항목이 무엇인지, 누가 요청했는지 등 자유롭게 적어 두세요. 콘솔에만 보이고 게이트웨이 설정에는 영향이 없습니다.",
  notesPlaceholder: "예: 쇼핑몰 프론트엔드, 담당: 웹팀",
};

type NotesKey = keyof typeof koNotes;

export const notesStrings: Record<"en" | "zh" | "ja" | "ru" | "fr" | "de" | "es", Record<NotesKey, string>> = {
  en: {
    secNotes: "Notes",
    notesHint: "Anything worth remembering: what this is for, who asked for it. Only shown in the console; it does not change the gateway configuration.",
    notesPlaceholder: "e.g. Shop frontend, owned by the web team",
  },
  zh: {
    secNotes: "备注",
    notesHint: "记录任何值得记住的信息：用途、谁要求添加的等。仅在控制台中显示，不影响网关配置。",
    notesPlaceholder: "例如：商城前端，负责人：Web 团队",
  },
  ja: {
    secNotes: "メモ",
    notesHint: "用途や依頼者など、覚えておきたいことを自由に書けます。コンソールにのみ表示され、ゲートウェイの設定には影響しません。",
    notesPlaceholder: "例: ショップのフロントエンド、担当: Web チーム",
  },
  ru: {
    secNotes: "Заметки",
    notesHint: "Всё, что стоит запомнить: для чего это и кто об этом просил. Видно только в консоли и не меняет конфигурацию шлюза.",
    notesPlaceholder: "например: фронтенд магазина, отвечает веб-команда",
  },
  fr: {
    secNotes: "Notes",
    notesHint: "Tout ce qui mérite d'être retenu : à quoi cela sert, qui l'a demandé. Visible uniquement dans la console ; ne modifie pas la configuration de la passerelle.",
    notesPlaceholder: "p. ex. Frontend de la boutique, géré par l'équipe web",
  },
  de: {
    secNotes: "Notizen",
    notesHint: "Alles, was man sich merken sollte: wofür es ist, wer es angefragt hat. Nur in der Konsole sichtbar; ändert die Gateway-Konfiguration nicht.",
    notesPlaceholder: "z. B. Shop-Frontend, betreut vom Web-Team",
  },
  es: {
    secNotes: "Notas",
    notesHint: "Cualquier cosa que convenga recordar: para qué sirve, quién lo pidió. Solo se muestra en la consola; no cambia la configuración del gateway.",
    notesPlaceholder: "p. ej. Frontend de la tienda, a cargo del equipo web",
  },
};
