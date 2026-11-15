// Warnings when changing the proxy host the console itself is opened through — 8 languages.
export const koConsoleHost = {
  consoleHostTitle: "지금 이 호스트를 통해 콘솔에 접속 중입니다",
  consoleHostEdit: "설정을 잘못 바꾸면 이 주소로는 콘솔에 들어올 수 없게 됩니다. 그럴 때는 서버의 9443 포트(https://서버-IP:9443)로 직접 접속하세요.",
  consoleHostDisableTitle: "콘솔이 쓰는 호스트를 끌까요?",
  consoleHostDisable: "이 호스트를 끄면 지금 주소로는 콘솔에 접속할 수 없습니다. 서버의 9443 포트(https://서버-IP:9443)로 직접 접속할 수 있는지 먼저 확인하세요.",
  consoleHostDelete: "지금 이 호스트를 통해 콘솔에 접속 중입니다. 삭제하면 이 주소로는 콘솔에 접속할 수 없습니다. 서버의 9443 포트(https://서버-IP:9443)로 직접 접속할 수 있는지 먼저 확인하세요.",
  consoleHostBadge: "콘솔 접속 중",
  disable: "끄기",
};

export type ConsoleHostKey = keyof typeof koConsoleHost;

/** True when the browser reached the console through one of these host names (wildcards included). */
export function servesConsole(domains: string[], host = typeof location === "undefined" ? "" : location.hostname): boolean {
  const h = host.toLowerCase();
  return domains.some((d) => {
    const v = d.toLowerCase();
    return v === h || (v.startsWith("*.") && h.endsWith(v.slice(1)) && h.length > v.length - 1);
  });
}
