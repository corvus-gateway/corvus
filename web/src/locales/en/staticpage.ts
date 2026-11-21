import type { StaticPageKey } from "../staticpage.i18n";

export const staticPageStrings: Record<StaticPageKey, string> = {
  spContent: "Content",
  spForward: "Forward to a service",
  spPage: "Serve a page",
  spPageDesc: "The gateway answers with a page itself, no backend needed. Security rules, Basic auth, rate limits, HTTPS, the access log and statistics still apply; custom locations keep forwarding to their backends.",
  spTemplate: "Template",
  spTplParked: "Parked domain",
  spTplComingSoon: "Coming soon",
  spTplMaintenance: "Maintenance",
  spTplCustom: "Custom HTML",
  spStatus: "HTTP status",
  spStatusHint: "503 is usual for maintenance: search engines come back later instead of indexing the page. Responses other than 200 are never cached.",
  spTitle: "Title",
  spMessage: "Message",
  spTextHint: "Leave empty for the default text, shown in the visitor's browser language. Your text is shown as entered (no HTML).",
  spHtmlHint: "Served exactly as entered. Put images and CSS into the page or load them from another site.",
  spPreviewHint: "Preview in the console language; visitors see the default texts in their browser language.",
  spPreviewFailed: "The preview could not be shown",
  spListPage: "Page: {name}",
  spHtmlNotLoaded: "The page's HTML could not be loaded. It stays as saved unless you enter new HTML here.",
};
