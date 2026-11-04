// Replacing an uploaded certificate (certificates page) — Korean source strings + translations.
export const koCertReplace = {
  replaceCertAction: "교체",
  replaceCertTitle: "인증서 교체",
  replaceCertDesc: "갱신한 인증서와 개인 키를 붙여 넣으세요. 인증서 ID와 호스트 연결은 그대로 유지되고, 이 인증서를 쓰는 호스트가 다시 배포됩니다.",
  certReplaced: "인증서를 교체했습니다",
};

type CertReplaceKey = keyof typeof koCertReplace;

export const certReplaceStrings: Record<"en" | "zh" | "ja" | "ru" | "fr" | "de" | "es", Record<CertReplaceKey, string>> = {
  en: {
    replaceCertAction: "Replace",
    replaceCertTitle: "Replace certificate",
    replaceCertDesc: "Paste the renewed certificate and private key. The certificate keeps its ID and host assignments; the hosts using it are redeployed.",
    certReplaced: "Certificate replaced",
  },
  zh: {
    replaceCertAction: "替换",
    replaceCertTitle: "替换证书",
    replaceCertDesc: "粘贴续期后的证书和私钥。证书保留其 ID 和主机关联，使用该证书的主机会重新部署。",
    certReplaced: "证书已替换",
  },
  ja: {
    replaceCertAction: "置き換え",
    replaceCertTitle: "証明書の置き換え",
    replaceCertDesc: "更新した証明書と秘密鍵を貼り付けてください。証明書の ID とホストの割り当てはそのままで、この証明書を使うホストが再デプロイされます。",
    certReplaced: "証明書を置き換えました",
  },
  ru: {
    replaceCertAction: "Заменить",
    replaceCertTitle: "Замена сертификата",
    replaceCertDesc: "Вставьте обновлённый сертификат и закрытый ключ. Сертификат сохраняет свой ID и привязки к хостам; хосты, которые его используют, будут развёрнуты заново.",
    certReplaced: "Сертификат заменён",
  },
  fr: {
    replaceCertAction: "Remplacer",
    replaceCertTitle: "Remplacer le certificat",
    replaceCertDesc: "Collez le certificat renouvelé et la clé privée. Le certificat garde son ID et ses hôtes ; les hôtes qui l'utilisent sont redéployés.",
    certReplaced: "Certificat remplacé",
  },
  de: {
    replaceCertAction: "Ersetzen",
    replaceCertTitle: "Zertifikat ersetzen",
    replaceCertDesc: "Erneuertes Zertifikat und privaten Schlüssel einfügen. Das Zertifikat behält seine ID und Host-Zuordnungen; die Hosts, die es verwenden, werden neu bereitgestellt.",
    certReplaced: "Zertifikat ersetzt",
  },
  es: {
    replaceCertAction: "Reemplazar",
    replaceCertTitle: "Reemplazar certificado",
    replaceCertDesc: "Pega el certificado renovado y la clave privada. El certificado conserva su ID y sus hosts asignados; los hosts que lo usan se vuelven a desplegar.",
    certReplaced: "Certificado reemplazado",
  },
};
