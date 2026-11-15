// Certificates page: replacing an uploaded certificate, failed renewals — Korean source strings + translations.
export const koCertReplace = {
  replaceCertAction: "교체",
  replaceCertTitle: "인증서 교체",
  replaceCertDesc: "갱신한 인증서와 개인 키를 붙여 넣으세요. 인증서 ID와 호스트 연결은 그대로 유지되고, 이 인증서를 쓰는 호스트가 다시 배포됩니다.",
  certReplaced: "인증서를 교체했습니다",
  renewFailed: "갱신 실패",
  renewFailedTip: "갱신에 실패했습니다. 만료될 때까지 기존 인증서를 계속 사용합니다. 오류: {e}",
};

export type CertReplaceKey = keyof typeof koCertReplace;
