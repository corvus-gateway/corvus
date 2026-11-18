// Data retention (traffic history, request log archives) — 8 languages.
export const koRetention = {
  navRetention: "데이터 보관",
  retentionDesc: "트래픽 통계·보안 이벤트·요청 로그를 얼마나 오래, 얼마만큼 보관할지 정합니다. 기간이나 용량을 넘은 데이터는 매시간 자동으로 정리됩니다.",
  retentionHow: "통계와 이벤트는 재시작 후에도 유지됩니다. 요청 로그는 날짜별 파일로 저장되며 지난 날짜 파일은 압축됩니다. 용량 한도를 넘으면 가장 오래된 날짜 파일부터 삭제합니다.",
  logMode: "요청 로그 저장",
  logModeOff: "저장 안 함", logModeNotable: "주요 요청만", logModeAll: "모든 요청",
  logModeDesc: "주요 요청: 4xx/5xx 응답, 차단, 보안 탐지, 접속 대기열 대기. '모든 요청'은 트래픽이 많으면 하루 수 GB가 될 수 있습니다.",
  logDays: "로그 보관 기간(일)", logMaxMb: "로그 최대 용량(MB)", logMaxHint: "넘으면 오래된 날짜 파일부터 삭제합니다.",
  statsDays: "통계 보관 기간(일)", eventsDays: "이벤트 보관 기간(일)",
  storageUsage: "저장 공간", dbSize: "통계·이벤트 DB", logSize: "요청 로그", logFilesN: "파일 {n}개", logLimit: "최대 {mb} MB",
  droppedLines: "디스크 기록이 밀려 저장하지 못한 로그 {n}줄",
  archives: "요청 로그 파일", noArchives: "저장된 로그 파일이 없습니다", noArchivesDesc: "요청 로그 저장을 켜면 날짜별 파일이 여기에 표시됩니다.",
  fileName: "파일", fileSize: "크기", fileModified: "수정 시각", retentionSaved: "보관 설정을 저장했습니다",
  logModeAllHint: "'모든 요청'은 Nginx Proxy Manager의 호스트별 액세스 로그에 해당하며, 방문자 통계(유입 출처·UTM 집계)에 필요합니다.",
  referrer: "유입 출처", combinedHint: "nginx combined 형식으로 내려받기 (GoAccess 등 로그 분석 도구용)",
} as const;

export type K = keyof typeof koRetention;
