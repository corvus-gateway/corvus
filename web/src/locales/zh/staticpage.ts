import type { StaticPageKey } from "../staticpage.i18n";

export const staticPageStrings: Record<StaticPageKey, string> = {
  spContent: "响应方式",
  spForward: "转发到服务",
  spPage: "显示页面",
  spPageDesc: "无需后端，由网关直接返回页面。安全规则、Basic 认证、速率限制、HTTPS、访问日志和统计照常生效；自定义位置仍转发到各自的后端。",
  spTemplate: "模板",
  spTplParked: "停放域名",
  spTplComingSoon: "即将推出",
  spTplMaintenance: "维护中",
  spTplCustom: "自定义 HTML",
  spStatus: "HTTP 状态码",
  spStatusHint: "维护时通常使用 503：搜索引擎会稍后再来，而不会索引该页面。200 以外的响应不会被缓存。",
  spTitle: "标题",
  spMessage: "消息",
  spTextHint: "留空则以访问者浏览器的语言显示默认文字。您输入的文字按原样显示（不支持 HTML）。",
  spHtmlHint: "按输入内容原样发送。请将图片和 CSS 内嵌到页面中，或从其他网站加载。",
  spPreviewHint: "以控制台语言显示的预览；访问者看到的默认文字为其浏览器语言。",
  spPreviewFailed: "无法显示预览",
  spListPage: "页面：{name}",
};
