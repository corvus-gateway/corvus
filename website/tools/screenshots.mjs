// Captures the console screenshots used by the documentation site.
//
// Run it against a Corvus instance that holds example data only (example.com names, no real
// addresses). It starts its own headless Chrome with a throwaway profile and uses no npm packages.
//
//   CORVUS_URL=https://127.0.0.1:9443 CORVUS_LOGIN=/path/to/login.txt \
//     node website/tools/screenshots.mjs [shot-name ...]
//
// login.txt holds the e-mail on the first line and the password on the second.
// Images are written to website/en/img and website/ko/img (console language en / ko).
// Optional: CHROME=<path to chrome or msedge>, LANGS=en,ko, WIDTH=1440, HEIGHT=900.

import { spawn } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, existsSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const base = (process.env.CORVUS_URL || "https://127.0.0.1:9443").replace(/\/$/, "");
const [email, password] = readFileSync(process.env.CORVUS_LOGIN, "utf8").split(/\r?\n/);
const langs = (process.env.LANGS || "en,ko").split(",");
const W = +(process.env.WIDTH || 1440), H = +(process.env.HEIGHT || 900);
const only = process.argv.slice(2);

// Helpers available to the `act` snippets below (evaluated in the page).
const pageHelpers = `
  window.__sleep = (ms) => new Promise(r => setTimeout(r, ms));
  window.__q = (sel) => document.querySelector(sel);
  window.__qa = (sel) => [...document.querySelectorAll(sel)];
  // first row of the main table that contains text t
  window.__row = (t) => __qa("table tbody tr").find(r => r.textContent.includes(t));
  // open the row's "..." menu and choose its first item (Edit)
  window.__edit = async (t) => {
    const b = __row(t).querySelector('[aria-haspopup="menu"]');
    b.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, button: 0, pointerType: "mouse" }));
    await __sleep(300);
    __q('[role="menuitem"]').click();
    await __sleep(700);
  };
  // scroll the open form so the element whose text matches re is at the top
  window.__scrollTo = async (re) => {
    const el = __qa("[role=dialog] *").find(e =>
      [...e.childNodes].some(n => n.nodeType === 3 && re.test(n.textContent.trim())));
    if (el) {
      el.scrollIntoView({ block: "start" });
      let p = el.parentElement; while (p && p.scrollHeight <= p.clientHeight) p = p.parentElement;
      if (p) p.scrollTop -= 16;
      await __sleep(300);
    }
    return !!el;
  };
`;

// name: file name; path: console route; act: optional JS run after load (may return a CSS
// selector to capture only that element); full: capture the whole scrollable page;
// dialog: capture only the open form panel; whole: keep the top bar and sidebar.
const shots = [
  { name: "login", path: "/", loggedOut: true, whole: true },
  { name: "dashboard", path: "/", whole: true },
  { name: "proxy-hosts", path: "/proxy-hosts" },
  { name: "proxy-host-form", path: "/proxy-hosts", act: `await __edit("shop.example.com");`, dialog: true },
  { name: "proxy-host-locations", path: "/proxy-hosts", act: `await __edit("shop.example.com"); await __scrollTo(/^(Custom locations|사용자 지정 위치)$/);`, dialog: true },
  { name: "proxy-host-tls", path: "/proxy-hosts", act: `await __edit("shop.example.com"); await __scrollTo(/^TLS$/);`, dialog: true },
  { name: "proxy-host-protection", path: "/proxy-hosts", act: `await __edit("shop.example.com"); await __scrollTo(/^(Protection|보호)$/);`, dialog: true },
  { name: "proxy-host-queue", path: "/proxy-hosts", act: `await __edit("shop.example.com"); await __scrollTo(/^(Access queue|접속 대기열)$/);`, dialog: true },
  { name: "proxy-host-advanced", path: "/proxy-hosts", act: `await __edit("shop.example.com"); await __scrollTo(/^(Advanced|고급)$/);`, dialog: true },
  { name: "certificates", path: "/certificates" },
  { name: "redirection-hosts", path: "/redirection-hosts" },
  { name: "streams", path: "/streams" },
  { name: "stream-form", path: "/streams", act: `await __edit("5432");`, dialog: true },
  { name: "security-rules", path: "/security-rules" },
  { name: "security-rule-form", path: "/security-rules", act: `await __edit("Challenge login");`, dialog: true },
  { name: "ip-lists", path: "/ip-lists" },
  { name: "bans", path: "/bans" },
  { name: "waitrooms", path: "/waitrooms" },
  { name: "waitroom-form", path: "/waitrooms", act: `__qa("button").find(b => /^(Edit|편집)$/.test(b.textContent.trim())).click(); await __sleep(700);`, dialog: true },
  { name: "traffic", path: "/traffic" },
  { name: "traffic-logs", path: "/traffic", act: `
      __qa("[role=tab], button").find(b => /^(Request logs|요청 로그)$/.test(b.textContent.trim())).click();
      await __sleep(1500);
      __q("table tbody tr").click(); await __sleep(500);` },
  { name: "alerts", path: "/alerts" },
  { name: "import", path: "/import" },
  { name: "retention", path: "/settings/retention" },
  { name: "backup", path: "/settings/retention", act: `
      const h = __qa("main *").find(e => [...e.childNodes].some(n => n.nodeType === 3 && /^(Backup & restore|백업 및 복원)$/.test(n.textContent.trim())));
      h.scrollIntoView({ block: "start" }); window.scrollBy(0, -80); await __sleep(300);` },
  { name: "pages", path: "/settings/pages" },
  { name: "unknown-hosts", path: "/settings/pages", act: `
    const h = __qa("main *").find(e => [...e.childNodes].some(n => n.nodeType === 3 && /^(Unknown hosts|알 수 없는 호스트)$/.test(n.textContent.trim())));
    let c = h; while (c && !/rounded|border/.test(c.className || "")) c = c.parentElement;
    c.id = "__uh";
    const set = (el, v) => { Object.getOwnPropertyDescriptor(el.constructor.prototype, "value").set.call(el, v); el.dispatchEvent(new Event(el.tagName === "SELECT" ? "change" : "input", { bubbles: true })); };
    set(c.querySelector("select"), "redirect"); await __sleep(300);
    set(c.querySelector("input"), "https://www.example.com/"); await __sleep(300);
    c.scrollIntoView({ block: "center" }); await __sleep(300); return "#__uh";` },
  { name: "deployments", path: "/deployments" },
  { name: "account", path: "/account" },
];

async function login() {
  process.env.NODE_TLS_REJECT_UNAUTHORIZED = "0"; // local instance with a self-signed certificate
  const r = await fetch(base + "/api/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Corvus-Request": "1" },
    body: JSON.stringify({ email, password }),
  });
  if (!r.ok) throw new Error("login failed: " + r.status);
  const c = r.headers.getSetCookie().map(s => s.split(";")[0].split("=")).find(([k]) => k);
  return { name: c[0], value: c.slice(1).join("=") };
}

function findChrome() {
  if (process.env.CHROME) return process.env.CHROME;
  const c = [
    "C:/Program Files/Google/Chrome/Application/chrome.exe",
    "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser",
  ];
  return c.find(existsSync);
}

async function cdpConnect(profile) {
  const port = join(profile, "DevToolsActivePort");
  for (let i = 0; i < 100 && !existsSync(port); i++) await new Promise(r => setTimeout(r, 100));
  const [p] = readFileSync(port, "utf8").split("\n");
  const targets = await (await fetch(`http://127.0.0.1:${p}/json/list`)).json();
  const ws = new WebSocket(targets.find(t => t.type === "page").webSocketDebuggerUrl);
  await new Promise((ok, bad) => { ws.onopen = ok; ws.onerror = bad; });
  let id = 0; const waiting = new Map();
  ws.onmessage = (m) => {
    const d = JSON.parse(m.data);
    if (d.id && waiting.has(d.id)) { const [ok, bad] = waiting.get(d.id); waiting.delete(d.id); d.error ? bad(new Error(d.error.message)) : ok(d.result); }
  };
  const send = (method, params = {}) => new Promise((ok, bad) => { const i = ++id; waiting.set(i, [ok, bad]); ws.send(JSON.stringify({ id: i, method, params })); });
  return { send, close: () => ws.close() };
}

const sleep = (ms) => new Promise(r => setTimeout(r, ms));

async function main() {
  const cookie = await login();
  const profile = mkdtempSync(join(tmpdir(), "corvus-shots-"));
  const chrome = spawn(findChrome(), [
    "--headless=new", "--remote-debugging-port=0", `--user-data-dir=${profile}`, "--no-first-run",
    "--ignore-certificate-errors", "--hide-scrollbars", `--window-size=${W},${H}`, "about:blank",
  ], { stdio: "ignore" });
  try {
    const { send, close } = await cdpConnect(profile);
    await send("Page.enable"); await send("Runtime.enable"); await send("Network.enable");
    await send("Emulation.setDeviceMetricsOverride", { width: W, height: H, deviceScaleFactor: 1, mobile: false });
    await send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: "light" }] });
    for (const lang of langs) {
      const out = join(here, "..", lang, "img");
      mkdirSync(out, { recursive: true });
      const boot = await send("Page.addScriptToEvaluateOnNewDocument", { source: `try{localStorage.setItem("corvus.lang","${lang}")}catch(e){}` + pageHelpers });
      for (const s of shots) {
        if (only.length && !only.includes(s.name)) continue;
        await send("Network.clearBrowserCookies");
        if (!s.loggedOut) await send("Network.setCookie", { ...cookie, url: base, secure: true, httpOnly: true });
        await send("Page.navigate", { url: base + s.path });
        await sleep(2500); // charts and live counters settle
        let clip;
        if (s.act) {
          const r = await send("Runtime.evaluate", { expression: `(async () => { ${s.act} })()`, awaitPromise: true, returnByValue: true });
          if (r.exceptionDetails) console.warn(`${lang}/${s.name}: ${r.exceptionDetails.exception?.description || r.exceptionDetails.text}`);
          await sleep(500);
          // a returned CSS selector: capture only that element (with a small margin)
          if (typeof r.result?.value === "string") {
            const e = await send("Runtime.evaluate", { expression: `JSON.stringify(document.querySelector(${JSON.stringify(r.result.value)})?.getBoundingClientRect())`, returnByValue: true });
            const b = e.result.value && JSON.parse(e.result.value);
            if (b) clip = { x: Math.max(0, b.x - 12), y: Math.max(0, b.y - 12), width: b.width + 24, height: b.height + 24, scale: 1 };
          }
        }
        const params = { format: "png" };
        // Page shots show the content area only (no top bar and sidebar), so text stays readable.
        if (!s.whole && !s.dialog) params.clip = { x: 240, y: 56, width: W - 240, height: H - 56, scale: 1 };
        if (s.dialog) {
          const r = await send("Runtime.evaluate", { expression: `JSON.stringify(document.querySelector("[role=dialog]")?.getBoundingClientRect())`, returnByValue: true });
          const b = r.result.value && JSON.parse(r.result.value);
          if (b) params.clip = { x: b.x, y: b.y, width: b.width, height: b.height, scale: 1 };
        }
        if (clip) params.clip = clip;
        if (s.full) {
          const m = await send("Page.getLayoutMetrics");
          params.clip = { x: 0, y: 0, width: W, height: Math.ceil(m.cssContentSize.height), scale: 1 };
          params.captureBeyondViewport = true;
        }
        const { data } = await send("Page.captureScreenshot", params);
        writeFileSync(join(out, s.name + ".png"), Buffer.from(data, "base64"));
        console.log(`${lang}/${s.name}.png`);
      }
      await send("Page.removeScriptToEvaluateOnNewDocument", { identifier: boot.identifier });
    }
    close();
  } finally {
    chrome.kill();
    await sleep(500);
    try { rmSync(profile, { recursive: true, force: true }); } catch { /* still locked on Windows */ }
  }
}

main().catch((e) => { console.error(e); process.exit(1); });
