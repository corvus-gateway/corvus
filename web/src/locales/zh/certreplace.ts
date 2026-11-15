import type { CertReplaceKey } from "../certreplace.i18n";

export const certReplaceStrings: Record<CertReplaceKey, string> = {
  replaceCertAction: "替换",
  replaceCertTitle: "替换证书",
  replaceCertDesc: "粘贴续期后的证书和私钥。证书保留其 ID 和主机关联，使用该证书的主机会重新部署。",
  certReplaced: "证书已替换",
  renewFailed: "续期失败",
  renewFailedTip: "续期失败；在过期前继续使用当前证书。错误：{e}",
};
