// Thin JSON client for the Corvus admin API.

export interface FieldError { field: string; code: string; message: string }

export class ApiError extends Error {
  status: number;
  kind: string;
  fields: FieldError[];
  output?: string;
  constructor(status: number, body: any) {
    super(body?.message ?? `HTTP ${status}`);
    this.status = status;
    this.kind = body?.error ?? "error";
    this.fields = body?.fields ?? body?.errors ?? [];
    this.output = body?.output;
  }
}

/** True when an endpoint is not (yet) implemented by the connected backend. */
export const isMissing = (e: unknown) => e instanceof ApiError && (e.status === 404 || e.status === 405 || e.status === 501);

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (method !== "GET") headers["X-Corvus-Request"] = "1";
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, {
    method, headers, credentials: "same-origin",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  let data: any = null;
  if (text) {
    try { data = JSON.parse(text); } catch {
      // A non-JSON 2xx means the route is not an API route on this backend (e.g. SPA fallback): treat as missing.
      if (res.ok) throw new ApiError(404, { error: "not_json", message: "unexpected response" });
      data = { message: text.slice(0, 200) };
    }
  }
  if (!res.ok) {
    const err = new ApiError(res.status, data);
    if (res.status === 401 && !path.startsWith("/api/auth/login")) window.dispatchEvent(new Event("corvus:unauthorized"));
    throw err;
  }
  return data as T;
}

/** GET that returns raw text (CSV/JSON export). */
export async function getText(path: string): Promise<string> {
  const res = await fetch(path, { credentials: "same-origin" });
  const text = await res.text();
  if (!res.ok) {
    let b: any = null;
    try { b = JSON.parse(text); } catch { /* not json */ }
    throw new ApiError(res.status, b);
  }
  return text;
}

export const api = {
  get: <T>(p: string) => request<T>("GET", p),
  post: <T>(p: string, b?: unknown) => request<T>("POST", p, b ?? {}),
  put: <T>(p: string, b: unknown) => request<T>("PUT", p, b),
  del: <T>(p: string) => request<T>("DELETE", p),
};

/** Extracts the deployed version number from a mutation response (`version` may be a number or a Version). */
export function deployedVersion(res: unknown): number | null {
  const v = (res as any)?.version;
  if (typeof v === "number") return v;
  if (v && typeof v === "object" && typeof v.id === "number") return v.id;
  return null;
}

export interface User {
  id: number; email: string; name: string; role: string; created_at?: string; last_login_at?: string; mfa_enabled?: boolean; disabled?: boolean;
}
export type LoginResult = User | { mfa_required: true; mfa_token: string };

export type ClientIPMode = "direct" | "cloudflare" | "trusted_proxies";

export interface ProxyHost {
  id: number; domains: string[]; forward_scheme: string; forward_host: string; forward_port: number;
  websocket: boolean; certificate_id: number | null; ssl_forced: boolean; http2: boolean;
  waitroom_id: number | null; waitroom_path: string; advanced_config: string; enabled: boolean;
  client_ip?: ClientIPMode; rate_limit_id?: number | null; access_list_id?: number | null; block_exploits?: boolean;
  body_inspection?: "off" | "log" | "block"; body_max_kb?: number;
  security_headers?: SecurityHeaders;
  /** null = global default (100 MB), 0 = unlimited. */
  max_upload_mb?: number | null;
  /** Upstream read/send timeout in seconds; null = default (60 s, WebSocket 1 h). */
  upstream_timeout_s?: number | null;
  gzip?: boolean;
  under_attack?: UnderAttackMode;
  under_attack_config?: UnderAttackConfig;
  locations?: ProxyLocation[];
  /** Read-only: usernames of the host's password protection (HTTP Basic auth). */
  basic_auth_users?: string[];
  created_at: string; updated_at: string;
}

/** Custom location of a proxy host: a path prefix routed to its own upstream. */
export interface ProxyLocation {
  path: string; forward_scheme: "http" | "https"; forward_host: string; forward_port: number;
  forward_path: string; advanced_config: string;
}

/** "Under attack" mode of a proxy host (gateway/lua/corvus/underattack.lua). */
export type UnderAttackMode = "off" | "on" | "auto";
export interface UnderAttackConfig {
  mode: "js" | "cookie"; difficulty: number; ttl_s: number; bind: "ip" | "ip_ua";
  rps: number; blocked_pct: number; window_s: number; duration_s: number;
}
/** GET /api/under-attack */
export interface UnderAttackStatus {
  host_id: number; host: string; mode: "on" | "auto"; active: boolean; since?: string; until?: string; reason?: string;
}

/** Security header preset of a proxy host (gateway/lua/corvus/headers.lua). */
export type SecurityHeaders = "off" | "basic" | "strict";

export type RedirectScheme = "auto" | "http" | "https";
export interface RedirectionHost {
  id: number; domains: string[]; forward_scheme: RedirectScheme; forward_domain: string; http_code: number; preserve_path: boolean;
  certificate_id: number | null; ssl_forced: boolean; http2: boolean; block_exploits: boolean; advanced_config: string; enabled: boolean;
  created_at: string; updated_at: string;
}

/** TCP/UDP port forwarding (GET /api/streams). */
export interface Stream {
  id: number; incoming_port: number; tcp_forwarding: boolean; udp_forwarding: boolean; forward_host: string; forward_port: number;
  certificate_id: number | null; enabled: boolean; created_at: string; updated_at: string;
}

export interface LiveStats { name: string; available: boolean; active: number; waiting: number; capacity: number }

export interface Waitroom {
  id: number; name: string; enabled: boolean; capacity: number; active_ttl: number; grant_ttl: number;
  queue_ttl: number; refresh: number; admit_rate: number; fail_mode: "open" | "closed";
  max_queue?: number; max_new_per_ip?: number; bind_ip?: boolean;
  created_at: string; updated_at: string; live?: LiveStats;
}

export interface Certificate {
  id: number; name: string; kind: "selfsigned" | "custom" | "acme" | string; domains: string[]; not_after: string; created_at: string;
  status?: "valid" | "pending" | "error"; last_error?: string; auto_renew?: boolean; challenge?: string; dns_credential_id?: number;
}

/** Saved DNS-01 provider credential; the secret is never returned. */
export interface DnsCredential { id: number; name: string; provider: string; used_by: number; created_at: string; updated_at: string }

export interface Version {
  id: number; created_at: string; created_by: number | null; email: string; status: string; message: string;
  test_output: string; rollback_of: number | null; dir: string; config_rev: number;
}

export interface DeploymentPage { versions: Version[]; dirty: boolean; total?: number; limit?: number; offset?: number }

export interface AuditEntry { id: number; ts: string; email: string; action: string; target: string; detail: unknown; ip: string }

export interface TrafficSummary {
  range: string; requests: number; rps: number; s2xx: number; s3xx: number; s4xx: number; s5xx: number; error_rate: number;
  p50: number; p95: number; p99: number; bytes_out: number; blocked: number; queued: number; unique_ips: number;
}

export interface Overview {
  proxy_hosts: number; proxy_hosts_enabled: number; waitrooms: number; certificates: number;
  active_version: Version | null; dirty: boolean; redis: boolean; rooms: LiveStats[]; version: string;
  gateway?: { running: boolean; uptime_s?: number };
  traffic?: TrafficSummary | null;
  certificates_expiring?: number;
}

export interface RateLimit {
  id: number; name: string; requests_per_second: number; burst: number; max_connections: number; status_code: number;
  dry_run: boolean; exempt_cidrs: string[]; enabled: boolean; hosts?: number;
}

export interface NetworkSettings {
  trusted_proxies: string[]; real_ip_header: string; real_ip_headers?: string[]; mask_ips?: boolean;
  cloudflare?: { ipv4: number; ipv6: number; updated_at?: string; source?: "builtin" | "remote" };
}

/** Basic-auth credential set (IP allow/deny moved to security rules). */
export interface AccessList {
  id: number; name: string;
  users: { username: string; password?: string }[]; hosts?: number;
}

export type RuleField =
  | "ip.src" | "ip.country" | "http.host" | "http.path" | "http.query" | "http.method"
  | "http.user_agent" | "http.user_agent_list" | "http.referer" | "http.header" | "http.scheme" | "http.body";
export type RuleOperator =
  | "in" | "not_in" | "eq" | "ne" | "contains" | "not_contains" | "starts_with" | "ends_with"
  | "matches" | "not_matches" | "exists" | "not_exists";
export interface RuleCondition { field: RuleField; name?: string; operator: RuleOperator; value: string[]; ci?: boolean }
export type RuleAction = "block" | "allow" | "log" | "challenge";
/** Rule event actions: rule actions plus the outcome of a challenge answer. */
export type RuleEventAction = RuleAction | "challenge_pass" | "challenge_fail";
export type ChallengeMode = "js" | "cookie";
export type ChallengeBind = "ip" | "ip_ua";
export interface SecurityRule {
  id: number; name: string; description: string; enabled: boolean; position: number;
  action: RuleAction; status_code: number; skip_basic_auth: boolean;
  challenge_mode: ChallengeMode; challenge_difficulty: number; challenge_ttl_s: number; challenge_bind: ChallengeBind;
  scope: "all" | "hosts"; host_ids: number[];
  expression: { groups: RuleCondition[][] };
  hits_24h?: number; last_hit_at?: string | null; created_at?: string; updated_at?: string;
}
export interface RuleEvent {
  t: number | string; rule_id: number; rule: string; action: RuleEventAction; host: string; ip: string;
  country?: string; method?: string; path?: string; ua?: string; status?: number;
}
export interface GeoipStatus { available: boolean; source?: string; updated_at?: string | null; ranges?: number; error?: string; attribution?: string }
export type ListKind = "ip" | "user_agent";
/** A list for security rules: IPs/CIDRs or User-Agent substrings; optionally fed from a URL (then read-only). */
export interface IpList {
  id: number; name: string; description: string; kind?: ListKind; items: string[]; rules?: number; created_at?: string; updated_at?: string;
  source_url?: string; refresh_s?: number; fetched_at?: string | null; checked_at?: string | null; fetch_error?: string; fetch_warning?: string;
}

export interface TimePoint {
  t: number | string; requests: number; s2xx: number; s3xx: number; s4xx: number; s5xx: number; blocked: number;
  p50: number; p95: number; p99: number; bytes_out: number;
}
export interface Timeseries { step_s: number; points: TimePoint[] }
export interface HostTraffic { host: string; requests: number; rps: number; s4xx: number; s5xx: number; error_rate: number; p95: number; bytes_out: number; blocked: number }
export interface PathTraffic { host: string; path: string; requests: number; errors: number; p95: number }
export interface LogLine { ts: number | string; host: string; method: string; path: string; status: number; rt_ms: number; bytes: number; ip: string; wr?: string; ua?: string; rule?: string }
export type EventKind = "rate_limited" | "blocked_exploit" | "server_error" | "waitroom_queue" | "waitroom_fail"
  | "exploit_detected" | "blocked_rule" | "rule_logged" | "blocked_body" | "body_detected" | "banned" | "rate_limit_dry_run"
  | "challenge_issued" | "challenge_passed" | "challenge_failed";
export interface TrafficEvent { ts: number | string; kind: EventKind; host: string; path: string; ip: string; count: number }

export interface NpmPreview {
  proxy_hosts: { npm_id: number; domains: string[]; forward?: string; conflict?: string | boolean }[];
  redirection_hosts?: { npm_id: number; domains: string[]; forward?: string; conflict?: string | boolean }[];
  streams?: { npm_id: number; incoming_port: number; forward_host: string; forward_port: number; tcp_forwarding: boolean; udp_forwarding: boolean; conflict?: string | boolean }[];
  certificates: { npm_id: number; name?: string; nice_name?: string; domains?: string[] }[];
  access_lists: { npm_id: number; name?: string }[];
  unknown_hosts?: { mode: "not_found" | "close" | "redirect"; redirect_url: string };
  warnings: string[];
}

export interface MfaSetup { secret: string; otpauth_url: string; qr_png: string }

export type AlertSeverity = "info" | "warning" | "critical";
export type AlertKind = "slack" | "mattermost" | "discord" | "teams" | "telegram" | "webhook";
export interface AlertChannel {
  id: number; name: string; kind: AlertKind; enabled: boolean; min_severity: AlertSeverity; events: string[]; target_hint?: string;
  created_at?: string; updated_at?: string;
}
export interface AlertRuleCfg { enabled: boolean; severity: AlertSeverity; threshold?: number; window_s?: number; min_requests?: number }
export interface AlertSettings { cooldown_s: number; send_resolved: boolean; language: "ko" | "en"; public_url: string; rules: Record<string, AlertRuleCfg> }
export interface AlertRuleInfo { id: string; default: AlertRuleCfg; params: string[]; unit?: string }
export interface AlertHistoryItem {
  id: number; ts: number | string; rule: string; severity: AlertSeverity; status: "firing" | "resolved" | "test"; key: string; title: string; message: string;
  results?: { channel_id: number; channel: string; ok: boolean; error?: string }[];
}
export interface AlertActive { rule: string; key: string; severity: AlertSeverity; title: string; text?: string; since: number | string }
