import type { DnsCredsKey } from "../dnscreds.i18n";

export const dnsCredsStrings: Record<DnsCredsKey, string> = {
  dnsCredsTitle: "DNS 認証情報",
  dnsCredsDesc: "DNS-01 証明書に使う DNS プロバイダーの API トークンです。一度保存すれば、複数の証明書とその更新で共有されます。シークレットは暗号化して保存され、再表示されません。",
  dnsCredsEmpty: "保存された認証情報はまだありません。DNS-01 証明書をリクエストするときに入力した認証情報がここに保存されます。",
  dnsCredAdd: "認証情報を追加",
  dnsCredAddDesc: "DNS プロバイダーと API トークンを一度保存すれば、DNS-01 証明書のリクエスト時に選ぶだけで済みます。",
  dnsCredUsedBy: "使用中の証明書",
  dnsCredSelect: "DNS 認証情報",
  dnsCredNew: "新しい認証情報を入力",
  dnsCredName: "認証情報の名前",
  dnsCredNameHint: "この名前で保存され、次の証明書でも選べます。空欄なら自動で名前が付きます。",
  dnsCredReplace: "シークレットを置き換え",
  dnsCredReplaceDesc: "新しい認証情報を入力してください。この認証情報を使う証明書は次回の更新から新しい値を使います。今すぐ再発行はされません。",
  dnsCredRename: "名前を変更",
  dnsCredSaved: "認証情報を保存しました",
  dnsCredDeleted: "認証情報を削除しました",
  dnsCredDeleteTitle: "DNS 認証情報を削除しますか？",
  dnsCredDeleteDesc: "保存されたシークレットが削除されます。元に戻せません。",
  dnsCredInUse: "証明書で使用中のため削除できません",
};
