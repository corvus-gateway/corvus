import type { ProxyProtocolKey } from "../proxyprotocol.i18n";

export const proxyProtocolStrings: Record<ProxyProtocolKey, string> = {
  ppTitle: "PROXY protocol を受け付ける",
  ppDesc: "TCP トンネルやロードバランサー（FRP、HAProxy）が PROXY protocol（v1/v2）ヘッダーでクライアント IP を渡す場合にオンにします。80/443 ポートはそのままで、ヘッダーを受け付ける別のポートを開きます。",
  ppHttpPort: "HTTP ポート",
  ppHttpsPort: "HTTPS ポート",
  ppPortInvalid: "1〜65535 のポートを入力してください。",
  ppTrusted: "信頼する PROXY protocol の送信元",
  ppTrustedHint: "ヘッダーを送るトンネルやロードバランサーのアドレスです。ここからの接続だけがヘッダーのクライアント IP を使います。上の信頼するプロキシとは別のリストなので、トンネルのアドレスをそちらに入れないでください。",
  ppHint: "トンネルの接続先をこれらのポートにし、docker-compose でポートを公開してください。トンネルが同じホストにある場合はループバックだけに公開し（例: '127.0.0.1:8443:8443'）、そうでなければファイアウォールでトンネルのアドレスだけを許可してください。",
};
