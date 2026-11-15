import type { CertReplaceKey } from "../certreplace.i18n";

export const certReplaceStrings: Record<CertReplaceKey, string> = {
  replaceCertAction: "置き換え",
  replaceCertTitle: "証明書の置き換え",
  replaceCertDesc: "更新した証明書と秘密鍵を貼り付けてください。証明書の ID とホストの割り当てはそのままで、この証明書を使うホストが再デプロイされます。",
  certReplaced: "証明書を置き換えました",
  renewFailed: "更新失敗",
  renewFailedTip: "更新に失敗しました。期限が切れるまで現在の証明書を使い続けます。エラー: {e}",
};
