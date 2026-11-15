// Saved DNS credentials (certificates page, DNS-01 requests) — Korean source strings + translations.
export const koDnsCreds = {
  dnsCredsTitle: "DNS 인증 정보",
  dnsCredsDesc: "DNS-01 인증서에 쓰는 DNS 업체 API 토큰입니다. 한 번 저장하면 여러 인증서와 갱신에서 함께 씁니다. 비밀 값은 암호화되어 저장되고 다시 표시되지 않습니다.",
  dnsCredsEmpty: "저장된 인증 정보가 없습니다. DNS-01 인증서를 요청할 때 입력한 인증 정보가 여기에 저장됩니다.",
  dnsCredAdd: "인증 정보 추가",
  dnsCredAddDesc: "DNS 업체와 API 토큰을 저장해 두면 DNS-01 인증서를 요청할 때 고르기만 하면 됩니다.",
  dnsCredUsedBy: "사용 인증서",
  dnsCredSelect: "DNS 인증 정보",
  dnsCredNew: "새 인증 정보 입력",
  dnsCredName: "인증 정보 이름",
  dnsCredNameHint: "이 이름으로 저장되어 다음 인증서에서도 고를 수 있습니다. 비워 두면 자동으로 정해집니다.",
  dnsCredReplace: "비밀 값 교체",
  dnsCredReplaceDesc: "새 인증 정보를 입력하세요. 이 인증 정보를 쓰는 인증서는 다음 갱신부터 새 값을 쓰며, 지금 다시 발급하지는 않습니다.",
  dnsCredRename: "이름 변경",
  dnsCredSaved: "인증 정보를 저장했습니다",
  dnsCredDeleted: "인증 정보를 삭제했습니다",
  dnsCredDeleteTitle: "DNS 인증 정보를 삭제할까요?",
  dnsCredDeleteDesc: "저장된 비밀 값이 지워지며 되돌릴 수 없습니다.",
  dnsCredInUse: "인증서에서 사용 중이라 삭제할 수 없습니다",
};

export type DnsCredsKey = keyof typeof koDnsCreds;
