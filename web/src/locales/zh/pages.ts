import type { K } from "../pages.i18n";

export const pagesStrings: Record<K, string> = {
  navPages: "拦截与错误页面",
  pagesDesc: "自定义安全规则、封禁或攻击特征拦截请求时显示的拦截页面，以及无法连接后端时（502/503/504）显示的错误页面。保存后立即部署到网关。",
  blockPage: "拦截页面", errorPage: "错误页面",
  blockPageDesc: "当请求被安全规则、自动或手动封禁、攻击特征拦截拒绝时显示。",
  errorPageDesc: "当代理主机的后端无响应（连接被拒绝、超时）或返回无效响应时显示。后端自身返回的错误响应会原样转发。",
  pageCustom: "自定义", pageBuiltin: "内置页面",
  pageHtml: "HTML", pageBytes: "{n} / {max} KB", pageTooLarge: "页面过大（最大 {max} KB）",
  pagePlaceholders: "占位符",
  pagePlaceholdersHint: "发送页面时替换为实际值。值会进行 HTML 转义。",
  pageVarStatus: "状态码", pageVarRid: "请求 ID", pageVarHost: "请求的网站（主机）", pageVarIp: "访问者 IP", pageVarTime: "时间",
  pagePreview: "预览", pagePreviewHint: "使用示例值显示。预览中不会运行脚本（包括内置页面的自动翻译）。",
  pageReset: "恢复默认", pageResetTitle: "恢复内置页面？",
  pageResetDesc: "自定义页面将被删除，并立即部署内置页面。", pageResetDone: "已恢复内置页面",
  unknownHosts: "未知主机",
  unknownHostsDesc: "设置当 HTTP 请求的主机名未配置为任何代理主机或重定向主机时 Corvus 如何响应。此类主机名的 HTTPS 连接始终在 TLS 握手阶段被拒绝。",
  unknownMode: "响应方式",
  unknownNotFound: "未找到页面（404）", unknownClose: "关闭连接", unknownRedirect: "重定向到 URL",
  unknownRedirectUrl: "重定向 URL",
  unknownRedirectHint: "以 http:// 或 https:// 开头的完整 URL。以 302 响应，不附加请求路径。",
  importUnknownHosts: "将 NPM 的默认站点设置应用于未知主机：{mode}",
};
