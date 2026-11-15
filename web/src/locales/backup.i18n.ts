// Backup & restore (data retention page) — 8 languages.
export const koBackup = {
  backupTitle: "백업 및 복원",
  backupDesc: "설정 DB·인증서·비밀 키를 하나의 압축 파일로 내려받거나, 백업 파일로 복원합니다.",
  backupSecretsTitle: "백업 파일에는 비밀 정보가 들어 있습니다",
  backupSecretsWarn: "인증서 개인 키와 DB의 비밀 값을 복호화하는 키가 포함됩니다. 비밀번호처럼 다루세요: 암호화된 곳에, 접근을 제한해 보관하세요.",
  backupPgNote: "이 설치는 PostgreSQL을 사용하므로 백업에는 인증서와 비밀 키만 들어갑니다. DB는 pg_dump로 따로 백업하세요.",
  backupExcluded: "포함되지 않는 항목: 생성된 설정 버전, 트래픽 기록, 요청 로그, 국가 DB(다시 생성되거나 다시 내려받습니다).",
  backupDownload: "백업 다운로드", backupStarted: "백업을 만드는 중입니다. 곧 다운로드가 시작됩니다.",
  restoreTitle: "백업에서 복원",
  restoreHow: "업로드한 파일을 검사한 뒤 복원 대기 상태로 둡니다. 다음 재시작 때 현재 설정·인증서·비밀 키를 백업 내용으로 바꾸며, 기존 파일은 데이터 디렉터리의 restore-backup-<시각>/ 에 보관됩니다.",
  restoreChoose: "백업 파일 선택", restoreUpload: "업로드 후 검사", restoreChecked: "백업 파일을 검사했습니다",
  restoreVersion: "Corvus 버전", restoreCreated: "생성 시각", restoreEngine: "데이터베이스", restoreFiles: "파일",
  restoreCerts: "인증서", restoreSecrets: "비밀 키",
  restoreNoDb: "이 백업에는 DB가 없습니다(PostgreSQL). DB는 pg_restore로 복원하세요.",
  restoreVersionDiffers: "Corvus {v}에서 만든 백업입니다(현재 {cur}). 필요하면 시작할 때 DB가 마이그레이션됩니다.",
  restoreConfirm: "다음 재시작 때 복원",
  restoreConfirmWarn: "현재 설정·인증서·비밀 키가 모두 백업 내용으로 바뀝니다. 지금부터 재시작 전까지 바꾼 설정은 사라집니다.",
  restoreCancel: "복원 취소", restoreStaged: "복원을 예약했습니다", restoreDiscarded: "복원을 취소했습니다",
  restorePending: "복원 준비 완료 — 컨테이너를 재시작하면 끝납니다",
  restorePendingDesc: "docker-compose.yml이 있는 디렉터리에서 아래 명령을 실행하세요. 재시작 전까지는 현재 설정으로 계속 동작합니다.",
} as const;

export type K = keyof typeof koBackup;
