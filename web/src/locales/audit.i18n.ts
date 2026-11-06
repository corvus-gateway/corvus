// Audit log labels added after the per-language label maps (lib/audit.ts and the
// *AuditLabels of each locale): areas and actions that would otherwise show in English.
import type { Lang } from "../i18n";

export const auditMore: Record<Lang, { dom: Record<string, string>; verb: Record<string, string> }> = {
  ko: {
    dom: { traffic: "트래픽", under_attack: "공격 대응 모드" },
    verb: {
      forbidden: "권한 없음", login_locked: "로그인 잠김", mfa_failed: "MFA 실패", mfa_setup_started: "MFA 설정 시작", mfa_disable_failed: "MFA 해제 실패",
      password_change_failed: "비밀번호 변경 실패", sso_login: "SSO 로그인", sso_failed: "SSO 로그인 실패", download: "다운로드", permanent: "영구 차단",
      acme_requested: "ACME 발급 요청", acme_renew: "ACME 갱신", replace: "교체", files_viewed: "설정 파일 조회", npm: "NPM에서 가져오기", check: "업데이트 확인",
      cloudflare_refresh: "Cloudflare IP 갱신", log_download: "로그 다운로드", logs_viewed: "로그 조회", activate: "켜짐", deactivate: "꺼짐",
      mfa_reset: "MFA 초기화", password_reset: "비밀번호 재설정",
    },
  },
  en: {
    dom: { traffic: "Traffic", under_attack: "Under attack mode" },
    verb: {
      forbidden: "access denied", login_locked: "sign-in locked", mfa_failed: "MFA failed", mfa_setup_started: "MFA setup started", mfa_disable_failed: "MFA disable failed",
      password_change_failed: "password change failed", sso_login: "SSO sign-in", sso_failed: "SSO sign-in failed", download: "downloaded", permanent: "made permanent",
      acme_requested: "ACME requested", acme_renew: "ACME renewal", replace: "replaced", files_viewed: "files viewed", npm: "from NPM", check: "update check",
      cloudflare_refresh: "Cloudflare IPs refreshed", log_download: "logs downloaded", logs_viewed: "logs viewed", activate: "turned on", deactivate: "turned off",
      mfa_reset: "MFA reset", password_reset: "password reset",
    },
  },
  zh: {
    dom: { traffic: "流量", under_attack: "受攻击模式" },
    verb: {
      forbidden: "无权限", login_locked: "登录已锁定", mfa_failed: "MFA 失败", mfa_setup_started: "开始设置 MFA", mfa_disable_failed: "停用 MFA 失败",
      password_change_failed: "修改密码失败", sso_login: "SSO 登录", sso_failed: "SSO 登录失败", download: "下载", permanent: "设为永久",
      acme_requested: "申请 ACME", acme_renew: "ACME 续期", replace: "替换", files_viewed: "查看配置文件", npm: "从 NPM 导入", check: "检查更新",
      cloudflare_refresh: "刷新 Cloudflare IP", log_download: "下载日志", logs_viewed: "查看日志", activate: "开启", deactivate: "关闭",
      mfa_reset: "重置 MFA", password_reset: "重置密码",
    },
  },
  ja: {
    dom: { traffic: "トラフィック", under_attack: "攻撃対応モード" },
    verb: {
      forbidden: "権限なし", login_locked: "サインインのロック", mfa_failed: "MFA 失敗", mfa_setup_started: "MFA 設定開始", mfa_disable_failed: "MFA 無効化の失敗",
      password_change_failed: "パスワード変更の失敗", sso_login: "SSO サインイン", sso_failed: "SSO サインイン失敗", download: "ダウンロード", permanent: "無期限化",
      acme_requested: "ACME 発行要求", acme_renew: "ACME 更新", replace: "置き換え", files_viewed: "設定ファイルの表示", npm: "NPM からインポート", check: "更新の確認",
      cloudflare_refresh: "Cloudflare IP の更新", log_download: "ログのダウンロード", logs_viewed: "ログの表示", activate: "オン", deactivate: "オフ",
      mfa_reset: "MFA リセット", password_reset: "パスワードリセット",
    },
  },
  ru: {
    dom: { traffic: "Трафик", under_attack: "Режим «под атакой»" },
    verb: {
      forbidden: "доступ запрещён", login_locked: "вход заблокирован", mfa_failed: "ошибка MFA", mfa_setup_started: "начата настройка MFA", mfa_disable_failed: "не удалось отключить MFA",
      password_change_failed: "не удалось сменить пароль", sso_login: "вход через SSO", sso_failed: "ошибка входа через SSO", download: "скачано", permanent: "сделано бессрочным",
      acme_requested: "запрошен ACME", acme_renew: "продление ACME", replace: "заменено", files_viewed: "просмотр файлов", npm: "из NPM", check: "проверка обновлений",
      cloudflare_refresh: "обновлены IP Cloudflare", log_download: "журналы скачаны", logs_viewed: "просмотр журналов", activate: "включён", deactivate: "выключен",
      mfa_reset: "сброс MFA", password_reset: "сброс пароля",
    },
  },
  fr: {
    dom: { traffic: "Trafic", under_attack: "Mode « sous attaque »" },
    verb: {
      forbidden: "accès refusé", login_locked: "connexion verrouillée", mfa_failed: "échec MFA", mfa_setup_started: "configuration MFA commencée", mfa_disable_failed: "échec de la désactivation MFA",
      password_change_failed: "échec du changement de mot de passe", sso_login: "connexion SSO", sso_failed: "échec de connexion SSO", download: "téléchargé", permanent: "rendu permanent",
      acme_requested: "ACME demandé", acme_renew: "renouvellement ACME", replace: "remplacé", files_viewed: "fichiers consultés", npm: "depuis NPM", check: "vérification des mises à jour",
      cloudflare_refresh: "IP Cloudflare actualisées", log_download: "journaux téléchargés", logs_viewed: "journaux consultés", activate: "activé", deactivate: "désactivé",
      mfa_reset: "MFA réinitialisée", password_reset: "mot de passe réinitialisé",
    },
  },
  de: {
    dom: { traffic: "Traffic", under_attack: "Angriffsmodus" },
    verb: {
      forbidden: "Zugriff verweigert", login_locked: "Anmeldung gesperrt", mfa_failed: "MFA fehlgeschlagen", mfa_setup_started: "MFA-Einrichtung begonnen", mfa_disable_failed: "MFA-Deaktivierung fehlgeschlagen",
      password_change_failed: "Passwortänderung fehlgeschlagen", sso_login: "SSO-Anmeldung", sso_failed: "SSO-Anmeldung fehlgeschlagen", download: "heruntergeladen", permanent: "dauerhaft gemacht",
      acme_requested: "ACME angefordert", acme_renew: "ACME-Erneuerung", replace: "ersetzt", files_viewed: "Dateien angesehen", npm: "aus NPM", check: "Update-Prüfung",
      cloudflare_refresh: "Cloudflare-IPs aktualisiert", log_download: "Logs heruntergeladen", logs_viewed: "Logs angesehen", activate: "eingeschaltet", deactivate: "ausgeschaltet",
      mfa_reset: "MFA zurückgesetzt", password_reset: "Passwort zurückgesetzt",
    },
  },
  es: {
    dom: { traffic: "Tráfico", under_attack: "Modo «bajo ataque»" },
    verb: {
      forbidden: "acceso denegado", login_locked: "inicio de sesión bloqueado", mfa_failed: "MFA fallida", mfa_setup_started: "configuración de MFA iniciada", mfa_disable_failed: "error al desactivar MFA",
      password_change_failed: "error al cambiar la contraseña", sso_login: "inicio de sesión SSO", sso_failed: "error de inicio de sesión SSO", download: "descargado", permanent: "hecho permanente",
      acme_requested: "ACME solicitado", acme_renew: "renovación ACME", replace: "reemplazado", files_viewed: "archivos consultados", npm: "desde NPM", check: "comprobación de actualizaciones",
      cloudflare_refresh: "IP de Cloudflare actualizadas", log_download: "registros descargados", logs_viewed: "registros consultados", activate: "activado", deactivate: "desactivado",
      mfa_reset: "MFA restablecida", password_reset: "contraseña restablecida",
    },
  },
};
