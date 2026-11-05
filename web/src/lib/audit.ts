import { Activity, ArrowLeftRight, Cable, Signpost, FileInput, Gauge, Globe, KeyRound, ListChecks, LogIn, Network, ShieldBan, ShieldCheck, UserCog, Users, type LucideIcon } from "lucide-react";
import type { Lang } from "../i18n";
import { zhAuditLabels } from "../locales/zh";
import { jaAuditLabels } from "../locales/ja";
import { ruAuditLabels } from "../locales/ru";
import { frAuditLabels } from "../locales/fr";
import { deAuditLabels } from "../locales/de";
import { esAuditLabels } from "../locales/es";
import type { Tone } from "../components/ui/display";

const dom: Partial<Record<Lang, Record<string, string>>> = {
  ko: {
    ban: "IP 차단", autoban: "자동 차단", sso: "SSO",
    proxy_host: "프록시 호스트", redirection_host: "리디렉션 호스트", stream: "스트림", waitroom: "접속 대기열", certificate: "인증서", dns_credential: "DNS 인증 정보", deploy: "배포", auth: "인증", setup: "초기 설정",
    rate_limit: "Rate Limit", access_list: "Basic 인증", security_rule: "보안 규칙", ip_list: "IP 목록", geoip: "국가 DB", settings: "설정", network: "네트워크", import: "가져오기", mfa: "MFA", user: "계정", token: "API 토큰", alert_channel: "알림 채널", alert_rules: "알림 규칙", backup: "백업", managed_rules: "관리형 규칙", page: "페이지", retention: "데이터 보존", unknown_hosts: "알 수 없는 호스트",
  },
  en: {
    ban: "IP ban", autoban: "Auto ban", sso: "SSO",
    proxy_host: "Proxy host", redirection_host: "Redirection host", stream: "Stream", waitroom: "Access queue", certificate: "Certificate", dns_credential: "DNS credential", deploy: "Deployment", auth: "Auth", setup: "Setup",
    rate_limit: "Rate limit", access_list: "Basic auth", security_rule: "Security rule", ip_list: "IP list", geoip: "Country DB", settings: "Settings", network: "Network", import: "Import", mfa: "MFA", user: "Account", token: "API token", alert_channel: "Alert channel", alert_rules: "Alert rules", backup: "Backup", managed_rules: "Managed rules", page: "Page", retention: "Data retention", unknown_hosts: "Unknown hosts",
  },
};
const verb: Partial<Record<Lang, Record<string, string>>> = {
  ko: {
    create: "생성", update: "수정", delete: "삭제", upload: "업로드", apply: "적용", rollback: "롤백", login: "로그인", logout: "로그아웃",
    login_failed: "로그인 실패", password_changed: "비밀번호 변경", admin_created: "관리자 생성", renew: "갱신", issue: "발급",
    enable: "활성화", disable: "비활성화", refresh: "새로고침", mfa_enabled: "MFA 활성화", mfa_disabled: "MFA 비활성화", acme: "ACME 발급", reorder: "순서 변경", auto: "(자동)", test: "테스트", reset: "초기화", restore_upload: "복원 파일 업로드", restore_confirm: "복원 확정", restore_discard: "복원 취소", restore_rejected: "복원 거부",
  },
  en: {
    create: "created", update: "updated", delete: "deleted", upload: "uploaded", apply: "applied", rollback: "rolled back", login: "signed in", logout: "signed out",
    login_failed: "sign-in failed", password_changed: "password changed", admin_created: "admin created", renew: "renewed", issue: "issued",
    enable: "enabled", disable: "disabled", refresh: "refreshed", mfa_enabled: "MFA enabled", mfa_disabled: "MFA disabled", acme: "ACME issued", reorder: "reordered", auto: "(automatic)", test: "tested", reset: "reset", restore_upload: "restore uploaded", restore_confirm: "restore confirmed", restore_discard: "restore discarded", restore_rejected: "restore rejected",
  },
};

/** Audit labels for languages other than ko/en come from the locale files. */
export interface AuditLabels { dom: Record<string, string>; verb: Record<string, string> }
const extra: Partial<Record<Lang, AuditLabels>> = { zh: zhAuditLabels, ja: jaAuditLabels, ru: ruAuditLabels, fr: frAuditLabels, de: deAuditLabels, es: esAuditLabels };

export function actionLabel(action: string, lang: Lang): string {
  const i = action.indexOf(".");
  if (i < 0) return action;
  const d = action.slice(0, i), v = action.slice(i + 1);
  const x = extra[lang];
  const dm = x?.dom ?? dom[lang] ?? dom.en!, vb = x?.verb ?? verb[lang] ?? verb.en!;
  return `${dm[d] ?? dom.en![d] ?? d} ${vb[v] ?? verb.en![v] ?? v.replace(/_/g, " ")}`;
}

export function actionMeta(action: string): { icon: LucideIcon; tone: Tone } {
  const d = action.split(".")[0];
  const v = action.split(".").slice(1).join(".");
  const tone: Tone = /delete|failed|disable/.test(v) ? "danger" : /rollback/.test(v) ? "warn" : /create|upload|issue|acme|admin_created/.test(v) ? "ok" : /apply/.test(v) ? "brand" : "neutral";
  const icon: Record<string, LucideIcon> = {
    proxy_host: Globe, redirection_host: Signpost, stream: Cable, waitroom: Users, certificate: ShieldCheck, dns_credential: KeyRound, deploy: ArrowLeftRight, auth: LogIn, setup: UserCog,
    rate_limit: Gauge, access_list: KeyRound, security_rule: ShieldBan, ip_list: ListChecks, geoip: Globe, network: Network, settings: Network, import: FileInput, mfa: KeyRound, token: KeyRound,
  };
  return { icon: icon[d] ?? Activity, tone };
}
