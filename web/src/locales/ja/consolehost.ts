import type { ConsoleHostKey } from "../consolehost.i18n";

export const consoleHostStrings: Record<ConsoleHostKey, string> = {
  consoleHostTitle: "このホスト経由でコンソールに接続しています",
  consoleHostEdit: "設定を誤ると、このアドレスからコンソールに入れなくなります。その場合はサーバーの 9443 番ポート（https://サーバーIP:9443）に直接接続してください。",
  consoleHostDisableTitle: "コンソールが使っているホストを無効にしますか？",
  consoleHostDisable: "このホストを無効にすると、現在のアドレスではコンソールに接続できなくなります。先にサーバーの 9443 番ポート（https://サーバーIP:9443）へ直接接続できることを確認してください。",
  consoleHostDelete: "このホスト経由でコンソールに接続しています。削除すると、現在のアドレスではコンソールに接続できなくなります。先にサーバーの 9443 番ポート（https://サーバーIP:9443）へ直接接続できることを確認してください。",
  consoleHostBadge: "コンソール",
  disable: "無効にする",
};
