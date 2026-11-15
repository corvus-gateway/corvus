// Virtual-patch templates and managed rule bundle — 8 languages.
export const koTemplates = {
  fromTemplate: "템플릿에서 추가",
  templatesTitle: "가상 패치 템플릿",
  templatesDesc: "알려진 CVE를 패치하기 전까지 막아 주는 규칙입니다. 처음에는 '로그'로 적용해 오탐이 없는지 확인한 뒤 '차단'으로 바꾸세요.",
  tplSevCritical: "심각", tplSevHigh: "높음",
  tplProducts: "대상", tplFalsePositives: "오탐 가능성",
  tplExemptList: "예외 IP 목록 (관리자 네트워크)", tplExemptNone: "예외 없음",
  tplExemptHint: "이 템플릿은 관리자 네트워크를 예외로 둘 수 있습니다. 지정하지 않으면 모든 IP에 적용됩니다.",
  tplApply: "규칙 만들기", tplCreated: "템플릿으로 규칙을 만들었습니다", tplSelect: "템플릿을 선택하세요",
  managedRules: "관리형 규칙",
  managedRulesDesc: "본문 검사 시그니처와 가상 패치 템플릿 묶음입니다. 서명이 검증된 새 버전만 적용됩니다.",
  mrVersion: "버전", mrSource: "출처", mrBuiltin: "내장", mrInstalled: "업데이트됨", mrSignatures: "시그니처", mrTemplates: "템플릿", mrChecked: "마지막 확인",
  mrUrl: "업데이트 URL", mrUrlHint: "서명은 같은 주소 끝에 .sig를 붙인 위치에서 받습니다 (https만 허용).",
  mrAuto: "매일 자동 확인", mrCheckNow: "지금 확인",
  mrUpload: "직접 업로드", mrUploadHint: "인터넷이 막힌 환경에서는 bundle.json과 bundle.json.sig 파일을 올리세요.",
  mrBundleFile: "번들 파일 (bundle.json)", mrSigFile: "서명 파일 (bundle.json.sig)",
  mrUploaded: "규칙 묶음을 적용했습니다", mrUpToDate: "확인했습니다 (새 버전이 있으면 적용됩니다)", mrSaved: "업데이트 설정을 저장했습니다",
} as const;

export type K = keyof typeof koTemplates;
