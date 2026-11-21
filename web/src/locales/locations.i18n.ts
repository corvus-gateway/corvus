// Custom locations in the proxy host sheet — Korean source strings + translations.
export const koLocations = {
  secLocations: "사용자 지정 위치",
  locationsDesc: "경로별로 다른 업스트림으로 보냅니다. 각 위치에도 호스트의 보호(보안 규칙, 속도 제한, Basic 인증, 접속 대기열)가 그대로 적용됩니다. 위치 /는 전달 대상을 대체합니다.",
  locationsCount: "위치 {n}개",
  locationOne: "위치 1개",
  addLocation: "위치 추가",
  locPath: "경로",
  locForwardPath: "전달 경로",
  locForwardPathHint: "선택 사항. 일치한 경로를 이 경로로 바꿔 전달합니다(예: /api → /v2/).",
  locAdvanced: "고급 설정(지시어만, 관리자 전용)",
};

export type LocationsKey = keyof typeof koLocations;
