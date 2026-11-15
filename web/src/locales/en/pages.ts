import type { K } from "../pages.i18n";

export const pagesStrings: Record<K, string> = {
  navPages: "Block & error pages",
  pagesDesc: "Customize the page shown when a security rule, ban or attack-pattern block refuses a request, and the page shown when a backend cannot be reached (502/503/504). Saving deploys to the gateway immediately.",
  blockPage: "Block page", errorPage: "Error page",
  blockPageDesc: "Shown when a request is refused by a security rule, an automatic or manual ban, or attack-pattern blocking.",
  errorPageDesc: "Shown when a proxy host's backend does not answer (connection refused, timeout) or sends an invalid response. Error responses sent by the backend itself are passed through unchanged.",
  pageCustom: "Custom", pageBuiltin: "Built-in",
  pageHtml: "HTML", pageBytes: "{n} / {max} KB", pageTooLarge: "The page is too large (max {max} KB)",
  pagePlaceholders: "Placeholders",
  pagePlaceholdersHint: "Replaced with the actual values when the page is served. Values are HTML-escaped.",
  pageVarStatus: "Status code", pageVarRid: "Request ID", pageVarHost: "Requested site (host)", pageVarIp: "Visitor IP", pageVarTime: "Time",
  pagePreview: "Preview", pagePreviewHint: "Shown with sample values. Scripts do not run in the preview (including the built-in pages' automatic translation).",
  pageReset: "Reset to default", pageResetTitle: "Restore the built-in page?",
  pageResetDesc: "The custom page is deleted and the built-in page is deployed immediately.", pageResetDone: "Built-in page restored",
  unknownHosts: "Unknown hosts",
  unknownHostsDesc: "What Corvus answers to an HTTP request for a host name that no proxy or redirection host serves. HTTPS connections for such names are always refused during the TLS handshake.",
  unknownMode: "Answer with",
  unknownNotFound: "Not found page (404)", unknownClose: "Close the connection", unknownRedirect: "Redirect to a URL",
  unknownRedirectUrl: "Redirect URL",
  unknownRedirectHint: "A full URL starting with http:// or https://. Answered with 302; the requested path is not appended.",
  importUnknownHosts: "Apply NPM's default site setting to unknown hosts: {mode}",
};
