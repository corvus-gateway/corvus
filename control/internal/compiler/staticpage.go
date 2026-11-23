package compiler

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"os"
	"path/filepath"
	"strings"

	"corvus/control/internal/store"
)

// Built-in static pages (parked, coming soon, maintenance) in the style of the gateway's
// block / error pages. The texts are rendered in English (or the console language for a
// preview); a small script shows them in the visitor's browser language. An operator's
// own title or message replaces the default and is never translated.

//go:embed pages/static.html
var staticPageSrc string

var staticPageTmpl = htmltemplate.Must(htmltemplate.New("static").Parse(staticPageSrc))

// PageHostMarker in a built-in page is replaced by the gateway with the request's host
// name (HTML-escaped, corvus/page.lua). Custom HTML is served as is.
const PageHostMarker = "{{HOST}}"

// pageTexts: per template and language the badge, heading and text.
var pageTexts = map[string]map[string][3]string{
	store.PageParked: {
		"en": {"Parked domain", "This domain is parked", "This domain is registered, but there is no website here yet."},
		"ko": {"주차된 도메인", "이 도메인은 주차되어 있습니다", "등록된 도메인이지만 아직 웹사이트가 없습니다."},
		"zh": {"停放域名", "此域名已停放", "此域名已注册，但这里还没有网站。"},
		"ja": {"パーキングドメイン", "このドメインはパーキング中です", "このドメインは登録済みですが、まだウェブサイトはありません。"},
		"ru": {"Припаркованный домен", "Этот домен припаркован", "Домен зарегистрирован, но сайта здесь пока нет."},
		"fr": {"Domaine parqué", "Ce domaine est parqué", "Ce domaine est enregistré, mais aucun site n’est encore publié ici."},
		"de": {"Geparkte Domain", "Diese Domain ist geparkt", "Diese Domain ist registriert, aber hier gibt es noch keine Website."},
		"es": {"Dominio aparcado", "Este dominio está aparcado", "Este dominio está registrado, pero todavía no hay ningún sitio web aquí."},
	},
	store.PageComingSoon: {
		"en": {"Coming soon", "Coming soon", "We are working on something new. Please check back soon."},
		"ko": {"준비 중", "곧 찾아옵니다", "새로운 사이트를 준비하고 있습니다. 조금만 기다려 주세요."},
		"zh": {"即将推出", "即将上线", "我们正在准备新的内容，敬请期待。"},
		"ja": {"近日公開", "近日公開", "新しいサイトを準備中です。もうしばらくお待ちください。"},
		"ru": {"Скоро", "Скоро открытие", "Мы готовим кое-что новое. Загляните чуть позже."},
		"fr": {"Bientôt", "Bientôt disponible", "Nous préparons quelque chose de nouveau. Revenez bientôt."},
		"de": {"Demnächst", "Demnächst verfügbar", "Wir arbeiten an etwas Neuem. Schauen Sie bald wieder vorbei."},
		"es": {"Próximamente", "Próximamente", "Estamos preparando algo nuevo. Vuelva pronto."},
	},
	store.PageMaintenance: {
		"en": {"Maintenance", "Down for maintenance", "This site is undergoing maintenance and will be back shortly. Thank you for your patience."},
		"ko": {"점검 중", "점검 중입니다", "더 나은 서비스를 위해 점검을 진행하고 있습니다. 잠시 후 다시 이용해 주세요."},
		"zh": {"维护中", "网站维护中", "本网站正在进行维护，很快就会恢复。感谢您的耐心等待。"},
		"ja": {"メンテナンス中", "メンテナンス中です", "現在メンテナンスを行っています。まもなく再開しますので、しばらくお待ちください。"},
		"ru": {"Техработы", "Идут технические работы", "На сайте проводятся технические работы, скоро он снова заработает. Спасибо за терпение."},
		"fr": {"Maintenance", "Site en maintenance", "Ce site est en cours de maintenance et sera bientôt de retour. Merci de votre patience."},
		"de": {"Wartung", "Wartungsarbeiten", "Diese Website wird gerade gewartet und ist in Kürze wieder erreichbar. Vielen Dank für Ihre Geduld."},
		"es": {"Mantenimiento", "En mantenimiento", "Este sitio está en mantenimiento y volverá a estar disponible en breve. Gracias por su paciencia."},
	},
}

type staticPageView struct {
	Template, Lang, HTMLLang string
	Pill, Title, Message     string
	Host                     string
	Texts                    map[string][3]string
	Own                      [2]bool // title / message set by the operator
}

// RenderStaticPage returns the HTML of a validated static page: custom HTML as is, a
// built-in template with its texts in lang (en when unknown). host is shown on built-in
// pages ("" = PageHostMarker, replaced by the gateway).
func RenderStaticPage(p store.StaticPage, lang, host string) ([]byte, error) {
	if p.Template == store.PageCustom {
		return []byte(p.HTML), nil
	}
	texts, ok := pageTexts[p.Template]
	if !ok {
		return nil, errors.New("unknown page template")
	}
	if _, ok := texts[lang]; !ok {
		lang = "en"
	}
	d := texts[lang]
	v := staticPageView{Template: p.Template, Lang: lang, HTMLLang: lang, Pill: d[0], Title: d[1], Message: d[2], Host: host, Texts: texts}
	if host == "" {
		v.Host = PageHostMarker
	}
	if p.Title != "" {
		v.Title, v.Own[0] = p.Title, true
	}
	if p.Message != "" {
		v.Message, v.Own[1] = p.Message, true
	}
	if v.Own[0] || v.Own[1] {
		v.HTMLLang = "" // the operator's text: language unknown
	}
	var b bytes.Buffer
	if err := staticPageTmpl.Execute(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// writeStaticPage writes the page of a host that serves one into the version directory
// (pages/host-<id>.html), so it is tested, activated and rolled back with the rest of
// the configuration and never placed into nginx configuration strings.
func writeStaticPage(dir string, h *store.ProxyHost) (string, error) {
	b, err := RenderStaticPage(h.Page, "en", "")
	if err != nil {
		return "", fmt.Errorf("proxy host %d: %w", h.ID, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		return "", err
	}
	file := filepath.Join(dir, "pages", fmt.Sprintf("host-%d.html", h.ID))
	if err := os.WriteFile(file, b, 0o644); err != nil {
		return "", err
	}
	return filepath.ToSlash(file), nil
}

// PreviewStaticPage renders a page as the console previews it: built-in texts in the
// console language and the host name filled in (escaped by the template).
func PreviewStaticPage(p store.StaticPage, lang, host string) ([]byte, error) {
	host = strings.TrimPrefix(strings.TrimSpace(host), "*.")
	if host == "" {
		host = "example.com"
	}
	return RenderStaticPage(p, lang, host)
}
