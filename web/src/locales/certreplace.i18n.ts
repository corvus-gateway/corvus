// Certificates page: replacing an uploaded certificate, failed renewals — Korean source strings + translations.
export const koCertReplace = {
  replaceCertAction: "교체",
  replaceCertTitle: "인증서 교체",
  replaceCertDesc: "갱신한 인증서와 개인 키를 붙여 넣으세요. 인증서 ID와 호스트 연결은 그대로 유지되고, 이 인증서를 쓰는 호스트가 다시 배포됩니다.",
  certReplaced: "인증서를 교체했습니다",
  renewFailed: "갱신 실패",
  renewFailedTip: "갱신에 실패했습니다. 만료될 때까지 기존 인증서를 계속 사용합니다. 오류: {e}",
};

type CertReplaceKey = keyof typeof koCertReplace;

export const certReplaceStrings: Record<"en" | "zh" | "ja" | "ru" | "fr" | "de" | "es", Record<CertReplaceKey, string>> = {
  en: {
    replaceCertAction: "Replace",
    replaceCertTitle: "Replace certificate",
    replaceCertDesc: "Paste the renewed certificate and private key. The certificate keeps its ID and host assignments; the hosts using it are redeployed.",
    certReplaced: "Certificate replaced",
    renewFailed: "Renewal failed",
    renewFailedTip: "Renewal failed; the current certificate is used until it expires. Error: {e}",
  },
  zh: {
    replaceCertAction: "替换",
    replaceCertTitle: "替换证书",
    replaceCertDesc: "粘贴续期后的证书和私钥。证书保留其 ID 和主机关联，使用该证书的主机会重新部署。",
    certReplaced: "证书已替换",
    renewFailed: "续期失败",
    renewFailedTip: "续期失败；在过期前继续使用当前证书。错误：{e}",
  },
  ja: {
    replaceCertAction: "置き換え",
    replaceCertTitle: "証明書の置き換え",
    replaceCertDesc: "更新した証明書と秘密鍵を貼り付けてください。証明書の ID とホストの割り当てはそのままで、この証明書を使うホストが再デプロイされます。",
    certReplaced: "証明書を置き換えました",
    renewFailed: "更新失敗",
    renewFailedTip: "更新に失敗しました。期限が切れるまで現在の証明書を使い続けます。エラー: {e}",
  },
  ru: {
    replaceCertAction: "Заменить",
    replaceCertTitle: "Замена сертификата",
    replaceCertDesc: "Вставьте обновлённый сертификат и закрытый ключ. Сертификат сохраняет свой ID и привязки к хостам; хосты, которые его используют, будут развёрнуты заново.",
    certReplaced: "Сертификат заменён",
    renewFailed: "Ошибка продления",
    renewFailedTip: "Продление не удалось; текущий сертификат используется до истечения срока. Ошибка: {e}",
  },
  fr: {
    replaceCertAction: "Remplacer",
    replaceCertTitle: "Remplacer le certificat",
    replaceCertDesc: "Collez le certificat renouvelé et la clé privée. Le certificat garde son ID et ses hôtes ; les hôtes qui l'utilisent sont redéployés.",
    certReplaced: "Certificat remplacé",
    renewFailed: "Échec du renouvellement",
    renewFailedTip: "Le renouvellement a échoué ; le certificat actuel est utilisé jusqu'à son expiration. Erreur : {e}",
  },
  de: {
    replaceCertAction: "Ersetzen",
    replaceCertTitle: "Zertifikat ersetzen",
    replaceCertDesc: "Erneuertes Zertifikat und privaten Schlüssel einfügen. Das Zertifikat behält seine ID und Host-Zuordnungen; die Hosts, die es verwenden, werden neu bereitgestellt.",
    certReplaced: "Zertifikat ersetzt",
    renewFailed: "Erneuerung fehlgeschlagen",
    renewFailedTip: "Erneuerung fehlgeschlagen; das aktuelle Zertifikat wird bis zum Ablauf weiter verwendet. Fehler: {e}",
  },
  es: {
    replaceCertAction: "Reemplazar",
    replaceCertTitle: "Reemplazar certificado",
    replaceCertDesc: "Pega el certificado renovado y la clave privada. El certificado conserva su ID y sus hosts asignados; los hosts que lo usan se vuelven a desplegar.",
    certReplaced: "Certificado reemplazado",
    renewFailed: "Error al renovar",
    renewFailedTip: "La renovación falló; se usa el certificado actual hasta que caduque. Error: {e}",
  },
};
