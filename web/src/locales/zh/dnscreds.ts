import type { DnsCredsKey } from "../dnscreds.i18n";

export const dnsCredsStrings: Record<DnsCredsKey, string> = {
  dnsCredsTitle: "DNS 凭据",
  dnsCredsDesc: "用于 DNS-01 证书的 DNS 服务商 API 令牌。保存一次后，多个证书及其续期共用。密钥加密存储，且不会再次显示。",
  dnsCredsEmpty: "尚无已保存的凭据。申请 DNS-01 证书时输入的凭据会保存在这里。",
  dnsCredAdd: "添加凭据",
  dnsCredAddDesc: "保存一次 DNS 服务商和 API 令牌，之后申请 DNS-01 证书时直接选择即可。",
  dnsCredUsedBy: "证书",
  dnsCredSelect: "DNS 凭据",
  dnsCredNew: "输入新凭据",
  dnsCredName: "凭据名称",
  dnsCredNameHint: "以此名称保存，下一个证书可直接选择。留空则自动命名。",
  dnsCredReplace: "替换密钥",
  dnsCredReplaceDesc: "输入新凭据。使用此凭据的证书将从下次续期开始使用新值，现在不会重新签发。",
  dnsCredRename: "重命名",
  dnsCredSaved: "凭据已保存",
  dnsCredDeleted: "凭据已删除",
  dnsCredDeleteTitle: "删除 DNS 凭据？",
  dnsCredDeleteDesc: "已保存的密钥将被删除，且无法撤销。",
  dnsCredInUse: "有证书正在使用，无法删除",
};
