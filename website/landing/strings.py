"""Texts of the product homepage (template.html), English and Korean.

Values are inserted into the template as-is, so they may contain HTML. Keep facts in line with the
docs (website/en/, website/ko/), and keep both languages in step.
"""

EN = {
    "title": "Corvus AWPG: open-source reverse proxy with a built-in WAF",
    "description": ("Free, open-source and self-hosted. Proxy hosts and free HTTPS as easy as Nginx Proxy "
                    "Manager, plus attack blocking, automatic bans, a bot challenge, live traffic and a "
                    "waiting room. One Docker container."),
    "og_locale": "en_US",
    "skip": "Skip to content",
    "home_label": "home",
    "nav_label": "Main",
    "menu": "Menu",
    "nav_features": "Features",
    "nav_compare": "Compare",
    "nav_docs": "Docs",
    "nav_cta": "Get started",

    "hero_pill": "Reverse proxy with a built-in WAF",
    "hero_h1": 'The reverse proxy<br class="br-lg"> that <span class="grad">defends your sites</span>',
    "hero_lead": ("Free, open-source and self-hosted. Proxy hosts and free HTTPS as easy as Nginx Proxy "
                  "Manager, plus attack blocking, automatic bans, a bot challenge, live traffic and a "
                  "waiting room. One Docker container."),
    "hero_cta1": "Install in 5 minutes",
    "hero_cta2": "View on GitHub",
    "trust_free": "No paid edition",
    "trust_langs": "8 languages",
    "alt_dashboard": ("The Corvus console dashboard: requests per second, error rate, latency, blocked "
                      "requests, live traffic chart and access queues"),

    "ov_eyebrow": "One console",
    "ov_h2": "Everything a public server needs, without the glue",
    "ov_sub": ("A reverse proxy for TLS, a WAF, fail2ban, a log viewer and a queue for launch day: "
               "Corvus is all of them, set up with safe defaults."),
    "t1": "Proxy hosts", "t1d": "Domains, locations, redirects, TCP/UDP streams",
    "t2": "Free HTTPS", "t2d": "Let's Encrypt with auto-renewal and wildcards",
    "t3": "Built-in WAF", "t3d": "Exploit paths and request bodies, on by default",
    "t4": "Automatic bans", "t4d": "Repeat attackers locked out, for longer each time",
    "t5": "Security rules", "t5d": "IP, country, path, header and User-Agent",
    "t6": "Waiting room", "t6d": "Admit up to your capacity, queue the rest",
    "t7": "Live traffic", "t7d": "Requests, errors, latency and request logs",
    "t8": "Alerts", "t8d": "Slack, Discord, Teams, Telegram and webhooks",

    "features_label": "Features",
    "alt_hosts": "The proxy hosts list in the Corvus console",
    "alt_check": "The proxy host form with a failed connection check explaining that the port refused the connection",
    "alt_bans": "Automatic ban settings: detections, time window, first and longest ban, exempt networks",
    "alt_rules": "The security rules list with block, allow and challenge rules and their matches in the last 24 hours",
    "alt_traffic": "The traffic monitor: requests by status, latency percentiles and per-host statistics",
    "alt_queues": "The access queues page with one active queue at 3% of its capacity",
    "alt_queue_form": "The access queue form: capacity, admission rate and expiry times",
    "alt_pages": "The block and error pages editor with a live preview",
    "alt_page_form": "The proxy host form set to serve a maintenance page with status 503",
    "f1_tag": "Proxy & HTTPS",
    "f1_h": "Add a site with free HTTPS in a minute",
    "f1_p": ("Enter the domain and where the app runs, then save. Corvus gets a Let's Encrypt "
             "certificate, renews it on time and deploys the change right away."),
    "f1_b1": "Wildcards, WebSocket, HTTP/2 and custom locations",
    "f1_b2": "A connection check that says why a backend can't be reached",
    "f1_b3": "Several backends per host, round-robin with failover",
    "f1_more": "Your first proxy host",

    "f2_tag": "Protection",
    "f2_h": "Protection is on from the first host",
    "f2_p": ("Known exploit paths and request bodies are checked the moment a host is added, without "
             "writing a single rule. IPs that keep attacking are banned automatically."),
    "f2_b1": "Ban durations that double for repeat offenders",
    "f2_b2": "Rate limits per host, exempt networks for your office",
    "f2_b3": "Under attack mode that challenges every visitor of a host",
    "f2_more": "How protection works",

    "f3_tag": "Rules & challenge",
    "f3_h": "Your own rules, and a bot check without a CAPTCHA",
    "f3_p": ("Block, allow, log or challenge requests by IP, country, path, header or User-Agent. The "
             "browser challenge is served by Corvus itself: no CAPTCHA provider, no third-party "
             "service."),
    "f3_b1": "Start from templates, try a rule in log-only mode first",
    "f3_b2": "IP and User-Agent lists, including blocklists with millions of entries",
    "f3_b3": "Country blocking with a monthly updated database",
    "f3_more": "Security rules",

    "f4_tag": "Monitoring",
    "f4_h": "See everything that reaches your sites",
    "f4_p": ("Live requests by status, latency percentiles, blocked requests and top paths for every "
             "host. Search the request logs, follow security events and get an alert when something "
             "is off."),
    "f4_b1": "Request logs you can search and export",
    "f4_b2": "Visitor statistics with referrers and UTM values",
    "f4_b3": "Alerts to Slack, Discord, Teams, Telegram and more",
    "f4_more": "Traffic monitor",

    "f5_tag": "Access queue",
    "f5_h": "A waiting room for launch day",
    "f5_p": ("Let in only as many visitors as your service can handle and queue the rest in order. "
             "Waiting visitors see their position on a page that refreshes by itself."),
    "f5_b1": "Capacity and admission rate you choose",
    "f5_b2": "Protect the whole site or only one path",
    "f5_b3": "Waiting page in 8 languages, emergency release in one click",
    "f5_more": "Access queue",

    "f6_tag": "Pages",
    "f6_h": "Serve a page without a backend",
    "f6_p": ("Coming soon, maintenance, a parked domain or your own HTML, answered by the gateway "
             "itself. Security rules, HTTPS and statistics still apply."),
    "f6_b1": "Ready-made templates in the visitor's language",
    "f6_b2": "The right status code, such as 503 for maintenance",
    "f6_b3": "Your own block and error pages, with a live preview",
    "f6_more": "Pages without a backend",

    "npm_tag": "Coming from NPM?",
    "npm_h": "Bring your Nginx Proxy Manager setup with you",
    "npm_p": ("Corvus reads an NPM data folder and imports proxy hosts, redirection hosts, streams, "
              "certificates and access lists. NPM itself is never changed, so you can go back at any "
              "time."),
    "npm_cta1": "Migration guide",
    "npm_cta2": "Corvus vs NPM",
    "npm_s1": "Mount the NPM data folder", "npm_s1d": "Read-only, into the Corvus container",
    "npm_s2": "Review the preview", "npm_s2d": "Conflicts and skipped items are listed",
    "npm_s3": "Pick what to import", "npm_s3d": "Hosts keep their certificates and access lists",

    "video_eyebrow": "Video",
    "video_h": "Watch the 2-minute intro",
    "video_p": ("Adding a site with free HTTPS, attack blocking and bans, security rules and the "
                "browser challenge, live traffic and the waiting room."),
    "video_aria": "Watch the 2-minute intro video on YouTube",
    "video_title": "Corvus in 2 minutes",
    "video_len": "YouTube · 1:43",

    "inst_eyebrow": "Install",
    "inst_h": "Up and running in five minutes",
    "inst_p": ("One container holds the gateway, the admin console and the queue store. All you need "
               "is a Linux server with Docker, amd64 or arm64."),
    "inst_s1": "Save the <code>docker-compose.yml</code> and run <code>docker compose up -d</code>.",
    "inst_s2": "Read the one-time setup token from the log.",
    "inst_s3": "Open <code>https://your-server:9443</code> and create the administrator.",
    "inst_cta": "Read the install guide",
    "inst_nas": "On a NAS? TrueNAS and Unraid app catalogs",
    "code_c1": "your sites",
    "code_c2": "admin console",
    "code_term": "Terminal",

    "tr_eyebrow": "Built in the open",
    "tr_h": "Software you can check, not just trust",
    "tr_sub": "A security tool should show its work. Here is how Corvus is built and shipped.",
    "tr1": "Signed releases",
    "tr1d": ("Images are signed with Sigstore cosign and ship with an SBOM and SLSA build provenance. "
             '<a href="/docs/reference/hardened/">Verify an image</a>.'),
    "tr2": "368 integration checks",
    "tr2d": ("Every pull request builds the image and runs 368 checks against a real gateway, plus "
             "Go, Lua and TypeScript tests and an upgrade test."),
    "tr3": "OpenSSF Best Practices",
    "tr3d": ('Listed on <a href="https://www.bestpractices.dev/projects/16315">OpenSSF Best '
             "Practices</a>, scored by OpenSSF Scorecard, CodeQL on every pull request."),
    "tr4": "Nothing leaves your server",
    "tr4d": ("Requests, logs and keys stay with you. No telemetry and no cloud account; the release "
             "check can be turned off."),
    "tr5": "AI-assisted, disclosed",
    "tr5d": ("Built with an AI coding assistant; the maintainer reviews every change. Where and how "
             'much: <a href="https://github.com/corvus-gateway/corvus/blob/main/AI-DECLARATION.md">AI '
             "declaration</a>."),
    "tr6": "Free, all of it",
    "tr6d": "Apache-2.0. No paid edition, no feature held back, no license key.",

    "cmp_eyebrow": "Compare",
    "cmp_h": "How Corvus compares",
    "cmp_sub": ("Good reverse proxies make different trade-offs. Here is where Corvus is different, "
                "and where the others fit better."),
    "cmp_caption": "Corvus compared with Nginx Proxy Manager, Caddy and Traefik",
    "cmp_feature": "Feature",
    "cmp_r1": "Managed in a web console",
    "cmp_r2": "WAF (attack signatures)",
    "cmp_r3": "Automatic IP bans",
    "cmp_r4": "Bot challenge without a CAPTCHA service",
    "cmp_r5": "Waiting room for traffic spikes",
    "cmp_r6": "Every feature free",
    "cmp_r7": "Configuration from Docker labels",
    "cmp_readonly": "Read-only dashboard",
    "cmp_plugin": "Plugin",
    "cmp_paid_or_plugin": "Paid or plugin",
    "cmp_hub": "Traefik Hub is paid",
    "cmp_note": "Checked against each project's documentation in November 2026.",
    "cmp_more": "Full comparison, including when Corvus is the wrong choice",
    "yes_sr": "Yes",
    "no_sr": "No",

    "final_h": "Put a shield in front of your apps today",
    "final_p": "One container, five minutes, and every feature included.",
    "final_cta2": "Read the docs",

    "foot_tag": "Open-source reverse proxy with a built-in WAF, automatic bans, live traffic and a waiting room.",
    "foot_label": "Footer",
    "foot_product": "Product",
    "foot_releases": "Releases",
    "foot_docs_home": "Documentation",
    "foot_install": "Install",
    "foot_migrate": "Migrate from NPM",
    "foot_security": "Security",
    "foot_community": "Community",
    "foot_discussions": "Discussions",
}

KO = {
    "title": "Corvus AWPG: 웹 방화벽을 내장한 오픈소스 리버스 프록시",
    "description": ("무료 오픈소스 셀프 호스팅 리버스 프록시. Nginx Proxy Manager처럼 쉬운 프록시 호스트와 무료 "
                    "HTTPS에 공격 차단, 자동 차단, 브라우저 확인, 트래픽 관제, 접속 대기열을 더했습니다. "
                    "Docker 컨테이너 하나로 동작합니다."),
    "og_locale": "ko_KR",
    "skip": "본문으로 건너뛰기",
    "home_label": "홈",
    "nav_label": "주 메뉴",
    "menu": "메뉴",
    "nav_features": "기능",
    "nav_compare": "비교",
    "nav_docs": "문서",
    "nav_cta": "시작하기",

    "hero_pill": "웹 방화벽(WAF) 내장 리버스 프록시",
    "hero_h1": '사이트를 <span class="grad">스스로 지키는</span><br class="br-lg"> 리버스 프록시',
    "hero_lead": ("무료 오픈소스 셀프 호스팅. Nginx Proxy Manager처럼 쉬운 프록시 호스트와 무료 HTTPS에 "
                  "공격 차단, 자동 차단, 브라우저 확인, 트래픽 관제, 접속 대기열을 더했습니다. "
                  "Docker 컨테이너 하나로 동작합니다."),
    "hero_cta1": "5분 만에 설치하기",
    "hero_cta2": "GitHub에서 보기",
    "trust_free": "유료 버전 없음",
    "trust_langs": "8개 언어",
    "alt_dashboard": "Corvus 콘솔 대시보드: 초당 요청, 오류율, 지연 시간, 차단된 요청, 실시간 트래픽 차트와 접속 대기열",

    "ov_eyebrow": "하나의 콘솔",
    "ov_h2": "공개 서버에 필요한 것을 한곳에",
    "ov_sub": ("TLS를 위한 리버스 프록시, 웹 방화벽, fail2ban, 로그 뷰어, 출시일을 위한 대기열까지. "
               "Corvus 하나로 안전한 기본값과 함께 갖출 수 있습니다."),
    "t1": "프록시 호스트", "t1d": "도메인, 사용자 지정 위치, 리디렉션, TCP/UDP 스트림",
    "t2": "무료 HTTPS", "t2d": "Let's Encrypt 자동 갱신과 와일드카드 인증서",
    "t3": "내장 웹 방화벽", "t3d": "공격 경로와 요청 본문을 기본으로 검사",
    "t4": "자동 차단", "t4d": "반복 공격 IP를 점점 더 길게 차단",
    "t5": "보안 규칙", "t5d": "IP, 국가, 경로, 헤더, User-Agent 조건",
    "t6": "접속 대기열", "t6d": "감당할 만큼만 입장, 나머지는 순서대로 대기",
    "t7": "트래픽 관제", "t7d": "요청, 오류, 지연 시간, 요청 로그",
    "t8": "알림", "t8d": "Slack, Discord, Teams, Telegram, 웹훅",

    "features_label": "기능",
    "alt_hosts": "Corvus 콘솔의 프록시 호스트 목록",
    "alt_check": "포트가 연결을 거부했다고 원인을 알려 주는 연결 확인 결과가 보이는 프록시 호스트 편집 화면",
    "alt_bans": "자동 차단 설정: 탐지 횟수, 기간, 첫 차단과 최장 차단 시간, 예외 네트워크",
    "alt_rules": "차단, 허용, 브라우저 확인 규칙과 최근 24시간 적중 수가 보이는 보안 규칙 목록",
    "alt_traffic": "트래픽 관제 화면: 상태별 요청, 지연 시간 백분위, 호스트별 통계",
    "alt_queues": "수용 인원의 3%가 입장한 접속 대기열 화면",
    "alt_queue_form": "접속 대기열 설정: 동시 입장 인원, 입장 속도, 만료 시간",
    "alt_pages": "미리 보기가 있는 차단·오류 페이지 편집 화면",
    "alt_page_form": "상태 코드 503으로 점검 중 페이지를 띄우도록 설정한 프록시 호스트 편집 화면",
    "f1_tag": "프록시와 HTTPS",
    "f1_h": "무료 HTTPS 사이트를 1분 만에",
    "f1_p": ("도메인과 앱 주소를 입력하고 저장하면 됩니다. Corvus가 Let's Encrypt 인증서를 받아 제때 "
             "갱신하고, 변경 사항을 바로 배포합니다."),
    "f1_b1": "와일드카드, WebSocket, HTTP/2, 사용자 지정 위치",
    "f1_b2": "백엔드에 연결되지 않는 이유를 알려 주는 연결 확인",
    "f1_b3": "호스트마다 여러 백엔드, 라운드 로빈과 장애 조치",
    "f1_more": "첫 프록시 호스트 만들기",

    "f2_tag": "보호",
    "f2_h": "첫 호스트부터 보호가 켜져 있습니다",
    "f2_p": ("호스트를 추가하는 순간부터 알려진 공격 경로와 요청 본문을 검사합니다. 규칙을 하나도 쓰지 "
             "않아도 됩니다. 공격을 반복하는 IP는 자동 차단합니다."),
    "f2_b1": "다시 걸리면 차단 시간이 두 배로 늘어납니다",
    "f2_b2": "호스트별 요청 속도 제한, 사무실 네트워크는 예외 처리",
    "f2_b3": "호스트의 모든 방문자를 확인하는 공격 대응 모드",
    "f2_more": "보호 동작 방식",

    "f3_tag": "보안 규칙과 브라우저 확인",
    "f3_h": "나만의 보안 규칙, CAPTCHA 없는 봇 확인",
    "f3_p": ("IP, 국가, 경로, 헤더, User-Agent로 요청을 차단, 허용, 기록하거나 브라우저 확인을 거치게 "
             "합니다. 브라우저 확인은 Corvus가 직접 제공하므로 CAPTCHA 서비스도, 외부 서비스도 필요 "
             "없습니다."),
    "f3_b1": "템플릿에서 시작하고, 기록만 하는 모드로 먼저 시험",
    "f3_b2": "수백만 항목의 차단 목록까지 지원하는 IP·User-Agent 목록",
    "f3_b3": "매달 갱신되는 데이터베이스로 국가 차단",
    "f3_more": "보안 규칙",

    "f4_tag": "트래픽 관제",
    "f4_h": "사이트에 들어오는 모든 것을 봅니다",
    "f4_p": ("호스트마다 상태별 실시간 요청, 지연 시간 백분위, 차단된 요청, 많이 요청된 경로를 보여 "
             "줍니다. 요청 로그를 검색하고 보안 이벤트를 따라가며, 이상이 있으면 알림을 받습니다."),
    "f4_b1": "검색하고 내보낼 수 있는 요청 로그",
    "f4_b2": "유입 경로와 UTM 값까지 보는 방문 통계",
    "f4_b3": "Slack, Discord, Teams, Telegram 등으로 알림",
    "f4_more": "트래픽 관제",

    "f5_tag": "접속 대기열",
    "f5_h": "출시일을 위한 접속 대기열",
    "f5_p": ("서비스가 감당할 수 있는 만큼만 방문자를 입장시키고 나머지는 순서대로 대기시킵니다. "
             "대기 중인 방문자는 스스로 새로 고쳐지는 페이지에서 자기 순번을 봅니다."),
    "f5_b1": "동시 입장 인원과 입장 속도를 직접 설정",
    "f5_b2": "사이트 전체 또는 특정 경로만 보호",
    "f5_b3": "8개 언어 대기 페이지, 클릭 한 번으로 긴급 해제",
    "f5_more": "접속 대기열",

    "f6_tag": "페이지",
    "f6_h": "백엔드 없이 페이지를 띄웁니다",
    "f6_p": ("오픈 예정, 점검 중, 주차된 도메인, 직접 만든 HTML을 게이트웨이가 직접 응답합니다. "
             "보안 규칙, HTTPS, 통계는 그대로 적용됩니다."),
    "f6_b1": "방문자 언어로 표시되는 기본 템플릿",
    "f6_b2": "점검 중에는 503처럼 알맞은 상태 코드",
    "f6_b3": "미리 보기와 함께 차단·오류 페이지를 직접 편집",
    "f6_more": "백엔드 없는 페이지",

    "npm_tag": "NPM을 쓰고 계신가요?",
    "npm_h": "Nginx Proxy Manager 설정을 그대로 가져옵니다",
    "npm_p": ("Corvus가 NPM 데이터 폴더를 읽어 프록시 호스트, 리디렉션 호스트, 스트림, 인증서, 접근 "
              "목록을 가져옵니다. NPM 자체는 바꾸지 않으므로 언제든 되돌아갈 수 있습니다."),
    "npm_cta1": "이전 가이드",
    "npm_cta2": "NPM과 비교",
    "npm_s1": "NPM 데이터 폴더 연결", "npm_s1d": "Corvus 컨테이너에 읽기 전용으로",
    "npm_s2": "미리 보기 확인", "npm_s2d": "충돌과 건너뛸 항목을 보여 줍니다",
    "npm_s3": "가져올 항목 선택", "npm_s3d": "호스트는 인증서·접근 목록과 함께 옮겨집니다",

    "video_eyebrow": "영상",
    "video_h": "2분 소개 영상",
    "video_p": ("무료 HTTPS 사이트 추가, 공격 차단과 자동 차단, 보안 규칙과 브라우저 확인, 트래픽 관제, "
                "접속 대기열을 2분 안에 보여 드립니다(영어)."),
    "video_aria": "YouTube에서 2분 소개 영상 보기",
    "video_title": "2분 만에 보는 Corvus",
    "video_len": "YouTube · 1:43 · 영어",

    "inst_eyebrow": "설치",
    "inst_h": "5분이면 동작합니다",
    "inst_p": ("게이트웨이, 관리 콘솔, 대기열 저장소가 컨테이너 하나에 들어 있습니다. Docker가 설치된 "
               "Linux 서버(amd64 또는 arm64)만 있으면 됩니다."),
    "inst_s1": "<code>docker-compose.yml</code>을 저장하고 <code>docker compose up -d</code>를 실행합니다.",
    "inst_s2": "로그에서 일회용 설정 토큰을 확인합니다.",
    "inst_s3": "<code>https://서버주소:9443</code>에 접속해 관리자를 만듭니다.",
    "inst_cta": "설치 가이드 보기",
    "inst_nas": "NAS를 쓰시나요? TrueNAS·Unraid 앱 카탈로그",
    "code_c1": "내 사이트",
    "code_c2": "관리 콘솔",
    "code_term": "터미널",

    "tr_eyebrow": "공개된 개발",
    "tr_h": "믿어 달라고 하지 않고, 확인할 수 있게 합니다",
    "tr_sub": "보안 도구라면 만드는 과정을 보여 줘야 합니다. Corvus는 이렇게 만들고 배포합니다.",
    "tr1": "서명된 릴리스",
    "tr1d": ("이미지는 Sigstore cosign으로 서명하고 SBOM과 SLSA 빌드 출처 증명을 함께 배포합니다. "
             '<a href="/ko/docs/reference/hardened/">이미지 검증 방법</a>.'),
    "tr2": "368개 통합 검사",
    "tr2d": ("모든 풀 리퀘스트마다 이미지를 빌드해 실제 게이트웨이에 368개 검사를 실행하고, "
             "Go·Lua·TypeScript 테스트와 업그레이드 테스트도 거칩니다."),
    "tr3": "OpenSSF Best Practices",
    "tr3d": ('<a href="https://www.bestpractices.dev/projects/16315">OpenSSF Best Practices</a>에 '
             "등록되어 있고, OpenSSF Scorecard로 평가받으며, 모든 풀 리퀘스트에서 CodeQL을 실행합니다."),
    "tr4": "서버 밖으로 나가지 않습니다",
    "tr4d": "요청, 로그, 키는 내 서버에 남습니다. 텔레메트리도 클라우드 계정도 없고, 새 버전 확인은 끌 수 있습니다.",
    "tr5": "AI 활용, 투명하게 공개",
    "tr5d": ("AI 코딩 도우미와 함께 개발하고, 모든 변경은 메인테이너가 검토합니다. 어디에 얼마나 "
             '쓰는지는 <a href="https://github.com/corvus-gateway/corvus/blob/main/AI-DECLARATION.md">AI '
             "활용 선언</a>에 공개합니다."),
    "tr6": "모든 기능 무료",
    "tr6d": "Apache-2.0. 유료 버전도, 막아 둔 기능도, 라이선스 키도 없습니다.",

    "cmp_eyebrow": "비교",
    "cmp_h": "다른 도구와 비교",
    "cmp_sub": "좋은 리버스 프록시도 저마다 장단점이 있습니다. Corvus가 다른 점과, 다른 도구가 더 잘 맞는 경우를 함께 보여 드립니다.",
    "cmp_caption": "Corvus와 Nginx Proxy Manager, Caddy, Traefik 비교",
    "cmp_feature": "기능",
    "cmp_r1": "웹 콘솔에서 관리",
    "cmp_r2": "웹 방화벽(공격 시그니처)",
    "cmp_r3": "자동 IP 차단",
    "cmp_r4": "CAPTCHA 서비스 없는 봇 확인",
    "cmp_r5": "트래픽 폭주에 대비한 접속 대기열",
    "cmp_r6": "모든 기능 무료",
    "cmp_r7": "Docker 라벨로 설정",
    "cmp_readonly": "읽기 전용 대시보드",
    "cmp_plugin": "플러그인",
    "cmp_paid_or_plugin": "유료 또는 플러그인",
    "cmp_hub": "Traefik Hub는 유료",
    "cmp_note": "2026년 11월 각 프로젝트 문서 기준입니다.",
    "cmp_more": "Corvus가 맞지 않는 경우까지 담은 전체 비교",
    "yes_sr": "지원",
    "no_sr": "미지원",

    "final_h": "지금 내 앱 앞에 방패를 세우세요",
    "final_p": "컨테이너 하나, 5분, 그리고 모든 기능이 포함되어 있습니다.",
    "final_cta2": "문서 보기",

    "foot_tag": "웹 방화벽, 자동 차단, 트래픽 관제, 접속 대기열을 갖춘 오픈소스 리버스 프록시.",
    "foot_label": "바닥글",
    "foot_product": "제품",
    "foot_releases": "릴리스",
    "foot_docs_home": "문서 홈",
    "foot_install": "설치",
    "foot_migrate": "NPM에서 이전",
    "foot_security": "보안",
    "foot_community": "커뮤니티",
    "foot_discussions": "Discussions",
}
