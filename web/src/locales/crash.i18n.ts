// A page that failed to render, and a server that cannot be reached — Korean source strings + translations.
export const koCrash = {
  crashTitle: "화면을 표시하지 못했습니다",
  crashDesc: "이 페이지를 그리는 중 오류가 발생했습니다. 다른 메뉴는 계속 사용할 수 있습니다. 새로 고침해도 반복되면 아래 내용을 알려 주세요.",
  crashUpdated: "콘솔이 새 버전으로 업데이트되었습니다. 새로 고침하면 계속 사용할 수 있습니다.",
  crashReload: "새로 고침",
  serverDownTitle: "서버에 연결할 수 없습니다",
  serverDownDesc: "Corvus가 다시 시작 중이거나 네트워크가 끊겼을 수 있습니다. 연결되면 자동으로 이어집니다.",
  serverDownRetry: "지금 다시 시도",
};

export type CrashKey = keyof typeof koCrash;
