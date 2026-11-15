import type { CertReplaceKey } from "../certreplace.i18n";

export const certReplaceStrings: Record<CertReplaceKey, string> = {
  replaceCertAction: "Replace",
  replaceCertTitle: "Replace certificate",
  replaceCertDesc: "Paste the renewed certificate and private key. The certificate keeps its ID and host assignments; the hosts using it are redeployed.",
  certReplaced: "Certificate replaced",
  renewFailed: "Renewal failed",
  renewFailedTip: "Renewal failed; the current certificate is used until it expires. Error: {e}",
};
