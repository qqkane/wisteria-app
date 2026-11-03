// Wisteria Desktop frontend — «Наша Сеть» design language.
// Vanilla JS over the Wails bindings (window.go.main.App.*), template
// rendering into a single wrapper (the overvpn-windows pattern).

"use strict";

const api = () => window.go && window.go.main && window.go.main.App;
function whenReady(fn, tries = 100) {
  if (api()) return fn();
  if (tries <= 0) { document.body.innerHTML = "<p style='padding:2em;color:#888'>Wails runtime unavailable</p>"; return; }
  setTimeout(() => whenReady(fn, tries - 1), 100);
}

// ---------- lab mode: static screenshot bridge (dev flag, never in prod) ----------
// ?lab=<disconnected|connecting|connected|reconnecting|error> swaps the
// Wails bindings for a deterministic mock, so every page/state renders in a
// plain browser (chromium --headless=new for the mission screenshots).
// ?lab=…&bg=<off|lilac|matrix|gradient> presets the ambient background;
// ?lab=…&open=<add|confirm> opens a modal. Production builds never carry
// the query parameter, and window.go is the real bridge.
const LAB = new URLSearchParams(location.search).get("lab") || null;

function labStatus(name) {
  const base = {
    profile_id: "p1", profile_name: "DE · Berlin", type: "wisteria",
    core: "wisteria-client", socks_addr: "127.0.0.1:2080", http_addr: "127.0.0.1:2081",
  };
  const started = new Date(Date.now() - 372 * 1000).toISOString();
  switch (name) {
    case "connected": return { ...base, state: "connected", pid: 4242, started_at: started, sys_proxy: true };
    case "connecting": return { ...base, state: "connecting" };
    case "reconnecting": return { ...base, state: "connecting", reconnecting: true, attempt: 3, sys_proxy: true, started_at: started,
      error: "соединение потеряно — автоматическое переподключение (задержка 4s)" };
    case "error": return { ...base, state: "disconnected", error: "порт 127.0.0.1:2080 так и не открылся (dial tcp 95.181.160.219:48200: connect: connection refused)" };
    default: return { state: "disconnected", socks_addr: "127.0.0.1:2080", http_addr: "127.0.0.1:2081" };
  }
}

function labBridge() {
  const st = labStatus(LAB);
  const bgParam = new URLSearchParams(location.search).get("bg") || "off";
  const profiles = [
    { id: "p1", type: "wisteria", name: "DE · Berlin", host: "g3x.owercone.ru", port: 48200, latency_ms: 38, sub_id: "" },
    { id: "p2", type: "wisteria", name: "FI · Helsinki", host: "3x2.owercone.ru", port: 48200, latency_ms: 91, sub_id: "" },
    { id: "p3", type: "vless", name: "NL · Amsterdam (REALITY)", host: "nl.example.net", port: 443, latency_ms: 52, sub_id: "s1" },
    { id: "p4", type: "ss", name: "DE · Shadowsocks", host: "de.example.net", port: 8388, latency_ms: 41, sub_id: "s1" },
    { id: "p5", type: "trojan", name: "SE · Stockholm", host: "se.example.net", port: 443, latency_ms: 67, sub_id: "s1" },
    { id: "p6", type: "hysteria2", name: "NL · Hy2", host: "hy.example.net", port: 443, latency_ms: -1, sub_id: "s1" },
  ];
  const subs = [{
    id: "s1", name: "моя подписка", url: "https://sub.example/subscribe",
    count: 4, updated_at: new Date(Date.now() - 3600e3).toISOString(),
    // demo of the link masking: the observer must redact the wisteria://
    // credential below to wisteria://•••• (the panel agent's loop-guard rule)
    error: "wisteria://UHdWeXpqVGtOZW1Qa3ZlTlczSk9AanVzdC5leGFtcGxlOjQ4MjAw?pad=64-1448&mimic=1 — последняя ссылка из подписки",
  }];
  const logs = [
    { seq: 1, at: new Date(Date.now() - 60e3).toISOString(), tag: "app", text: "Wisteria Desktop 1.1.6 запущено" },
    { seq: 2, at: new Date(Date.now() - 52e3).toISOString(), tag: "app", text: "запуск wisteria-client (pid 4242) для g3x.owercone.ru:48200 [wisteria]" },
    { seq: 3, at: new Date(Date.now() - 51e3).toISOString(), tag: "core", text: "connected to g3x.owercone.ru:48200" },
    { seq: 4, at: new Date(Date.now() - 51e3).toISOString(), tag: "core", text: "SOCKS5 listening on 127.0.0.1:2080 (via g3x.owercone.ru:48200)" },
    { seq: 5, at: new Date(Date.now() - 50e3).toISOString(), tag: "app", text: "подключено: g3x.owercone.ru:48200 [wisteria] через wisteria-client" },
    { seq: 6, at: new Date(Date.now() - 20e3).toISOString(), tag: "core", text: "health: rtt=6ms loss=0% pings=144" },
    { seq: 7, at: new Date(Date.now() - 5e3).toISOString(), tag: "app", text: "HTTP-мост на http://127.0.0.1:2081 → socks://127.0.0.1:2080" },
  ];
  const diagEvents = [
    { at: Date.now() - 90e3, kind: "up", text: "Подключено — туннель работает", raw: "connected to g3x.owercone.ru:48200" },
    { at: Date.now() - 60e3, kind: "health", text: "Состояние канала: rtt=6ms loss=0% pings=139", raw: "health: rtt=6ms loss=0% pings=139" },
    { at: Date.now() - 30e3, kind: "down", text: "Сессия прервана — ядро переподключается", raw: "session to g3x.owercone.ru:48200 ended" },
    { at: Date.now() - 29e3, kind: "retry", text: "Переподключение: retrying in 1s", raw: "dial failed: connection refused; retrying in 1s" },
    { at: Date.now() - 27e3, kind: "up", text: "Подключено — туннель работает", raw: "CONNECTED via g3x.owercone.ru:48200" },
  ];
  const diag = {
    active: st.state !== "disconnected",
    status: st,
    servers: [
      { endpoint: "g3x.owercone.ru:48200", rtt_ms: 38, reachable: true, failures: 0 },
      { endpoint: "3x2.owercone.ru:48200", rtt_ms: 91, reachable: true, failures: 1 },
    ],
    updated: Date.now() * 1e6,
    events: diagEvents,
    last_kind: st.reconnecting ? "retry" : "up",
  };
  const settings = {
    language: "ru", theme: "lilac", background: bgParam,
    local_port: 2080, http_port: 2081, sys_proxy: true, bridge_enabled: true,
    auto_connect: "p1", start_minimized: false, confirm_exit: true, check_ext_ip: true,
    auto_ping_sec: 60, no_update_check: false, listen_all: false, bypass_lan: true,
    remote_dns: "tcp://8.8.8.8", log_level: "warn", mem_limit_mb: 0,
    tun_enabled: false, tun_stack: "mixed", tun_strict_route: false,
    route_direct: "example.local", route_block: "", mp_core: "xray",
    core_ss: "xray", core_vmess: "xray", core_vless: "xray", core_trojan: "xray",
  };
  const ok = (v) => Promise.resolve(v);
  return {
    GetState: () => ok(st),
    GetWisteriaDiag: () => ok(diag),
    GetLogs: (since) => ok(logs.filter((l) => l.seq > (since || 0))),
    ListProfiles: () => ok(profiles),
    GetActiveID: () => ok("p1"),
    ListSubscriptions: () => ok(subs),
    GetSettings: () => ok(settings),
    SaveSettings: (s) => ok(s),
    DetectCores: () => ok({ wisteria_found: true, wisteria_path: "/opt/wisteria/bin/wisteria-client", singbox_found: false, singbox_path: "", xray_found: true, xray_path: "/opt/wisteria/bin/xray" }),
    IsElevated: () => ok(false),
    GetAutostart: () => ok(true),
    SetAutostart: () => ok(null),
    Version: () => ok("1.1.6"),
    CheckAppUpdate: () => ok({ has_update: false, current: "1.1.6", latest: "1.1.6" }),
    CheckCoreUpdate: () => ok({ has_update: false, current: "24.9.1" }),
    GetExternalIP: () => ok("94.228.171.70"),
    PingProfile: () => ok(42),
    Connect: () => ok(null),
    Disconnect: () => ok(null),
    SetActive: () => ok(null),
    DeleteProfile: () => ok(null),
    AddLinks: () => ok({ added: [], errors: [] }),
    ImportFromClipboard: () => Promise.reject(new Error("в буфере нет ни ссылки-подписки, ни share-ссылок")),
  };
}

if (LAB) {
  window.go = { main: { App: labBridge() } };
  window.runtime = { EventsOn: () => {}, EventsEmit: () => {}, LogErrorf: () => {} };
  // Masking for the screenshots (wisteria:// = credentials): replace, then
  // IGNORE our own mutation — an observer reacting to its own edit is the
  // infinite-loop trap the panel agent hit (guard flag, not debounce).
  setTimeout(() => {
    let redacting = false;
    const rx = /wisteria:\/\/[^\s"'<>]+/g;
    const doRedact = () => {
      if (redacting) return;
      redacting = true;
      const w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
      let n;
      while ((n = w.nextNode())) {
        const t2 = n.nodeValue.replace(rx, "wisteria://••••");
        if (t2 !== n.nodeValue) n.nodeValue = t2;
      }
      setTimeout(() => { redacting = false; }, 0);
    };
    new MutationObserver(() => { if (!redacting) doRedact(); })
      .observe(document.body, { childList: true, characterData: true, subtree: true });
    doRedact();
  }, 0);
  const labOpen = new URLSearchParams(location.search).get("open");
  if (labOpen === "add") setTimeout(() => openModal(), 400);
  if (labOpen === "confirm") setTimeout(() => askConfirm(I18N.ru["confirm.delete"]), 400);
  const labTab = new URLSearchParams(location.search).get("tab");
  if (labTab) setTimeout(() => switchTab(labTab), 350);
  // Backgrounds: advance the sim headlessly so a single headless screenshot
  // shows a mid-motion frame (rAF is throttled/paused in the screenshot env).
  setTimeout(() => { for (let i = 0; i < 200; i++) bgDraw(1 / 30); }, 500);
  // ?bench=1: time the preset's draw() synchronously — the per-frame CPU
  // cost numbers quoted in the mission journal come from these runs.
  if (new URLSearchParams(location.search).get("bench")) {
    setTimeout(() => {
      // Date.now(), not performance.now(): the screenshot harness runs on a
      // virtual clock, which freezes performance.now() inside one task.
      const t0 = Date.now();
      const N = 600;
      for (let i = 0; i < N; i++) bgDraw(1 / 30);
      const ms = (Date.now() - t0) / N;
      const bg = new URLSearchParams(location.search).get("bg") || "off";
      console.log("BG-BENCH " + bg + " " + ms.toFixed(3) + " ms/frame canvas=" + BG.w + "x" + BG.h + " sim=" + (BG.sim ? "ok" : "null") + " id=" + BG.id);
    }, 700);
  }
}

// ---------- inline svg icons (no CDN: the app must work offline) ----------
const I = {
  shield: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>',
  layers: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>',
  terminal: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>',
  gear: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>',
  power: '<svg class="power-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18.36 6.64a9 9 0 1 1-12.73 0"/><line x1="12" y1="2" x2="12" y2="12"/></svg>',
  plus: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>',
  globe: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>',
  trash: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',
  check: '<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10"/><polyline points="8 12.5 11 15.5 16 9.5" fill="none" stroke="#000" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>',
  xmark: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>',
  chev: '<svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>',
  ping: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M2 12h4l3-8 4 16 3-8h6"/></svg>',
  clock: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>',
  lock: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>',
  refresh: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>',
  clipboard: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/><rect x="8" y="2" width="8" height="4" rx="1" ry="1"/></svg>',
  activity: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>',
};

const THEMES = [
  { id: "lilac", name: "Lilac", color: "#a78bfa" },
  { id: "neon-dark", name: "Neon Dark", color: "#00e8a2" },
  { id: "warm-sand", name: "Warm Sand", color: "#e8a040" },
  { id: "arctic", name: "Arctic", color: "#2a64b4" },
  { id: "sunset", name: "Sunset", color: "#f06888" },
  { id: "matrix", name: "Matrix", color: "#00ff41" },
  { id: "mono", name: "Mono Red", color: "#e02020" },
];

// ---------- i18n ----------
const I18N = {
  ru: {
    "brand": "Wisteria",
    "nav.vpn": "VPN", "nav.profiles": "Профили", "nav.logs": "Логи", "nav.diag": "Диагностика", "nav.settings": "Настройки",
    "vpn.unprotected": "UNPROTECTED", "vpn.protected": "SECURED", "vpn.connecting": "CONNECTING…",
    "vpn.connect": "Connect", "vpn.disconnect": "Disconnect", "vpn.connected": "Connected",
    "vpn.noProfile": "Профиль не выбран", "vpn.clickSelect": "Нажмите, чтобы выбрать",
    "stat.uptime": "Аптайм", "stat.proxy": "Системный прокси", "stat.core": "Ядро",
    "profiles.title": "Профили", "profiles.empty": "Нет профилей — добавьте ссылку или TOML-конфиг",
    "settings.title": "Настройки", "sec.appearance": "Оформление", "sec.connection": "Соединение",
    "sec.general": "Общие", "sec.routing": "Маршрутизация и DNS", "sec.cores": "Ядра", "sec.system": "Система",
    "set.lang": "Язык", "set.socksPort": "Локальный SOCKS-порт", "set.httpPort": "Локальный HTTP-порт",
    "set.sysproxy": "Системный прокси при подключении", "set.bridge": "HTTP-мост (нужен для системного прокси)",
    "set.autoconnect": "Автоподключение при старте", "set.autostart": "Запускать при входе в Windows",
    "set.wisteriaCore": "Путь к wisteria-client", "set.singboxCore": "Путь к sing-box",
    "set.detect": "Найти ядра", "set.save": "Сохранить настройки",
    "set.startMin": "Запускать свёрнутым", "set.startMinNote": "окно свернётся в панель задач",
    "set.confirmExit": "Спрашивать подтверждение при выходе во время соединения",
    "set.checkIp": "Показывать внешний IP", "set.checkIpNote": "запрос через туннель после подключения",
    "set.autoping": "Автопинг серверов, сек", "set.autopingNote": "0 — выключено",
    "set.listenAll": "Разрешить доступ из локальной сети", "set.listenAllNote": "прокси слушает 0.0.0.0 вместо 127.0.0.1",
    "set.bypassLan": "Обходить локальные адреса",
    "set.remoteDns": "Удалённый DNS", "set.remoteDnsNote": "например tcp://8.8.8.8",
    "set.logLevel": "Уровень логирования",
    "set.memLimit": "Лимит памяти ядра, МБ", "set.memLimitNote": "0 — без лимита",
    "set.checkIpNow": "Проверить IP сейчас",
    "toast.ipFail": "Не удалось определить IP",
    "set.tun": "Режим TUN (весь трафик системы)",
    "set.tunNote": "маршрутизирует всё через туннель; нужен sing-box и wintun.dll; системный прокси при этом не используется",
    "set.tunStack": "Стек TUN",
    "set.tunStrict": "Строгий маршрут (strict route)", "set.tunStrictNote": "надёжнее против утечек; может мешать доступу к локальной сети",
    "set.tabLook": "Внешний вид", "set.tabGeneral": "Основные", "set.tabConn": "Подключение", "set.tabRouting": "Маршрутизация", "set.tabCores": "Ядра",
    "set.routeDirect": "Напрямую, минуя прокси", "set.routeDirectNote": "домены и IP через туннель не пойдут; по одному в строке",
    "set.routeBlock": "Блокировать", "set.routeBlockNote": "эти домены и IP будут недоступны",
    "set.coreSS": "Ядро для Shadowsocks", "set.coreVmess": "Ядро для VMess", "set.coreVless": "Ядро для VLESS", "set.coreTrojan": "Ядро для Trojan",
    "sec.engines": "Ядро по протоколу",
    "sec.updates": "Обновления",
    "set.updCheck": "Проверять обновления при запуске", "set.updNote": "приложение и ядра sing-box / xray",
    "set.appVersion": "Версия приложения",
    "set.checkUpd": "Проверить обновления", "set.updAvail": "Доступна версия",
    "set.updApply": "Скачать и установить", "set.updRestart": "Перезапустить приложение",
    "set.updCore": "Обновить", "set.dlCore": "Скачать",
    "toast.updNone": "актуальная версия",
    "toast.updReady": "Обновление загружено — можно перезапускать",
    "set.restartAdmin": "Перезапустить от администратора",
    "set.notAdmin": "Без прав администратора TUN недоступен.",
    "set.mpCore": "Ядро для ss/vmess/vless/trojan",
    "set.xrayCore": "Путь к xray",
    "logs.title": "Логи", "logs.follow": "Автопрокрутка", "logs.clear": "Очистить",
    "diag.title": "Диагностика wisteria", "diag.inactive": "Диагностика работает только для профилей wisteria",
    "diag.inactiveHint": "Подключитесь к профилю wisteria — здесь появится причина сбоя: заблокировали, упал интернет или ключ не принят.",
    "diag.bUp": "Туннель работает",
    "diag.bBlocked": "Сервер отклонил рукопожатие: ключ недействителен, ссылка устарела или сервер заблокирован",
    "diag.bNonet": "Сервер недоступен: упал интернет, сервер не отвечает или провайдер блокирует к нему доступ",
    "diag.bRetry": "Идёт переподключение — ядро повторяет попытки",
    "diag.bDown": "Соединение закрыто",
    "diag.attempts": "Попыток", "diag.bestRtt": "Лучший пинг", "diag.available": "Доступно серверов",
    "diag.servers": "Серверы", "diag.endpoint": "Адрес", "diag.rtt": "Задержка", "diag.reach": "Доступен", "diag.fails": "Ошибок",
    "diag.events": "События", "diag.noEvents": "Событий пока нет", "diag.openLogs": "Открыть логи",
    "add.title": "Добавить профиль", "tab.links": "Ссылки", "tab.manual": "Вручную", "tab.toml": "Wisteria TOML",
    "add.linksLabel": "Share-ссылки (по одной в строке)",
    "add.linksPh": "vless://…\nss://…\ntrojan://…\nhysteria2://…\ntuic://…\nwisteria://…",
    "add.namePh": "auto", "add.import": "Импортировать", "add.add": "Добавить",
    "toml.hint": "Вставьте client.toml — endpoint, ключи и обфускация переносятся; приватный ключ клиента сохраняется как есть.",
    "lbl.type": "Тип", "lbl.name": "Имя", "lbl.host": "Адрес", "lbl.port": "Порт",
    "toast.added": "Добавлено", "toast.saved": "Настройки сохранены",
    "confirm.delete": "Удалить профиль?",
    "subs.title": "Подписки", "subs.ph": "URL подписки (https://…)", "subs.add": "Добавить",
    "subs.added": "Подписка добавлена", "subs.updated": "Обновлена", "subs.never": "ещё не обновлялась",
    "subs.refresh": "Обновить", "subs.confirmDelete": "Удалить подписку и её профили?",
    "subs.emptyGroup": "Пусто — обновите подписку",
    "btn.clip": "Из буфера", "btn.clipTip": "Импортировать ссылки и подписки из буфера обмена",
    "profiles.manual": "Мои профили",
    "confirm.title": "Удаление", "confirm.cancel": "Отмена", "confirm.deleteBtn": "Удалить",
    "toast.busy": "Идёт подключение — секунду",
    "toast.clipSubs": "подписок: {n}", "toast.clipProfiles": "профилей: {n}", "toast.clipNone": "Ничего нового в буфере", "vpn.tunOn": "TUN активен — весь трафик системы идёт через туннель",
    "toast.stuck": "Подключение зависло — попробуйте ещё раз",
    "vpn.reconnecting": "RECONNECTING…",
    "vpn.autoReconnect": "Соединение потеряно — автоматическое переподключение (попытка {n})",
    "set.bgTitle": "Фон", "bg.off": "Выключен", "bg.lilac": "Сиреневое боке", "bg.matrix": "Матрица", "bg.gradient": "Градиент",
    "set.bgNote": "Ненавязчивая анимация с потолком ~30 fps; ставится на паузу, когда окно не в фокусе, и полностью отключается при prefers-reduced-motion.",
  },
  en: {
    "brand": "Wisteria",
    "nav.vpn": "VPN", "nav.profiles": "Profiles", "nav.logs": "Logs", "nav.diag": "Diagnostics", "nav.settings": "Settings",
    "vpn.unprotected": "UNPROTECTED", "vpn.protected": "SECURED", "vpn.connecting": "CONNECTING…",
    "vpn.connect": "Connect", "vpn.disconnect": "Disconnect", "vpn.connected": "Connected",
    "vpn.noProfile": "No profile selected", "vpn.clickSelect": "Click to choose",
    "stat.uptime": "Uptime", "stat.proxy": "System proxy", "stat.core": "Core",
    "profiles.title": "Profiles", "profiles.empty": "No profiles yet — add a link or a TOML config",
    "settings.title": "Settings", "sec.appearance": "Appearance", "sec.connection": "Connection",
    "sec.general": "General", "sec.routing": "Routing & DNS", "sec.cores": "Cores", "sec.system": "System",
    "set.lang": "Language", "set.socksPort": "Local SOCKS port", "set.httpPort": "Local HTTP port",
    "set.sysproxy": "Set system proxy on connect", "set.bridge": "HTTP bridge (required for the system proxy)",
    "set.autoconnect": "Auto-connect on start", "set.autostart": "Start with Windows",
    "set.wisteriaCore": "Path to wisteria-client", "set.singboxCore": "Path to sing-box",
    "set.detect": "Detect cores", "set.save": "Save settings",
    "set.startMin": "Start minimized", "set.startMinNote": "the window opens minimized",
    "set.confirmExit": "Ask for confirmation on exit while connected",
    "set.checkIp": "Show external IP", "set.checkIpNote": "queried through the tunnel after connecting",
    "set.autoping": "Auto-ping servers, sec", "set.autopingNote": "0 — disabled",
    "set.listenAll": "Allow LAN access", "set.listenAllNote": "proxy listens on 0.0.0.0 instead of 127.0.0.1",
    "set.bypassLan": "Bypass local addresses",
    "set.remoteDns": "Remote DNS", "set.remoteDnsNote": "e.g. tcp://8.8.8.8",
    "set.logLevel": "Log level",
    "set.memLimit": "Core memory limit, MB", "set.memLimitNote": "0 — unlimited",
    "set.checkIpNow": "Check IP now",
    "toast.ipFail": "Could not determine the IP",
    "set.tun": "TUN mode (all system traffic)",
    "set.tunNote": "routes everything through the tunnel; needs sing-box and wintun.dll; the system proxy is not used",
    "set.tunStack": "TUN stack",
    "set.tunStrict": "Strict route", "set.tunStrictNote": "stronger leak protection; may break LAN access",
    "set.tabLook": "Appearance", "set.tabGeneral": "General", "set.tabConn": "Connection", "set.tabRouting": "Routing", "set.tabCores": "Cores",
    "set.routeDirect": "Direct, bypassing the tunnel", "set.routeDirectNote": "domains and IPs that skip the tunnel; one per line",
    "set.routeBlock": "Blocked", "set.routeBlockNote": "these domains and IPs become unreachable",
    "set.coreSS": "Engine for Shadowsocks", "set.coreVmess": "Engine for VMess", "set.coreVless": "Engine for VLESS", "set.coreTrojan": "Engine for Trojan",
    "sec.engines": "Engine per protocol",
    "sec.updates": "Updates",
    "set.updCheck": "Check for updates on launch", "set.updNote": "the app and the sing-box / xray cores",
    "set.appVersion": "App version",
    "set.checkUpd": "Check for updates", "set.updAvail": "New version available",
    "set.updApply": "Download and install", "set.updRestart": "Restart the app",
    "set.updCore": "Update", "set.dlCore": "Download",
    "toast.updNone": "up to date",
    "toast.updReady": "Update staged — restart to apply",
    "set.restartAdmin": "Restart as administrator",
    "set.notAdmin": "TUN needs administrator rights.",
    "set.mpCore": "Engine for ss/vmess/vless/trojan",
    "set.xrayCore": "Path to xray",
    "logs.title": "Logs", "logs.follow": "Auto-scroll", "logs.clear": "Clear",
    "diag.title": "Wisteria diagnostics", "diag.inactive": "Diagnostics only works for wisteria profiles",
    "diag.inactiveHint": "Connect to a wisteria profile — the failure reason will show up here: server blocked, internet down, or the key was rejected.",
    "diag.bUp": "Tunnel is working",
    "diag.bBlocked": "The server rejected the handshake: the key is invalid, the link is stale, or the server is blocked",
    "diag.bNonet": "Server unreachable: your internet may be down, the server is not responding, or your ISP blocks it",
    "diag.bRetry": "Reconnecting — the core keeps retrying",
    "diag.bDown": "Connection closed",
    "diag.attempts": "Attempts", "diag.bestRtt": "Best RTT", "diag.available": "Servers reachable",
    "diag.servers": "Servers", "diag.endpoint": "Endpoint", "diag.rtt": "RTT", "diag.reach": "Reachable", "diag.fails": "Failures",
    "diag.events": "Events", "diag.noEvents": "No events yet", "diag.openLogs": "Open logs",
    "add.title": "Add profile", "tab.links": "Links", "tab.manual": "Manual", "tab.toml": "Wisteria TOML",
    "add.linksLabel": "Share links (one per line)",
    "add.linksPh": "vless://…\nss://…\ntrojan://…\nhysteria2://…\ntuic://…\nwisteria://…",
    "add.namePh": "auto", "add.import": "Import", "add.add": "Add",
    "toml.hint": "Paste your client.toml — endpoint, keys and obfuscation are carried over; the client private key is kept as is.",
    "lbl.type": "Type", "lbl.name": "Name", "lbl.host": "Host", "lbl.port": "Port",
    "toast.added": "Added", "toast.saved": "Settings saved",
    "confirm.delete": "Delete this profile?",
    "subs.title": "Subscriptions", "subs.ph": "Subscription URL (https://…)", "subs.add": "Add",
    "subs.added": "Subscription added", "subs.updated": "Updated", "subs.never": "never refreshed",
    "subs.refresh": "Refresh", "subs.confirmDelete": "Delete the subscription and its profiles?",
    "subs.emptyGroup": "Empty — refresh the subscription",
    "btn.clip": "From clipboard", "btn.clipTip": "Import links and subscriptions from the clipboard",
    "profiles.manual": "My profiles",
    "confirm.title": "Delete", "confirm.cancel": "Cancel", "confirm.deleteBtn": "Delete",
    "toast.busy": "Connection in progress — one moment",
    "toast.clipSubs": "subscriptions: {n}", "toast.clipProfiles": "profiles: {n}", "toast.clipNone": "Nothing new in the clipboard", "vpn.tunOn": "TUN active — all system traffic goes through the tunnel",
    "toast.stuck": "Connection is stuck — please try again",
    "vpn.reconnecting": "RECONNECTING…",
    "vpn.autoReconnect": "Connection lost — auto-reconnecting (attempt {n})",
    "set.bgTitle": "Background", "bg.off": "Off", "bg.lilac": "Lilac bokeh", "bg.matrix": "Matrix", "bg.gradient": "Gradient",
    "set.bgNote": "Unobtrusive animation capped at ~30 fps; pauses when the window loses focus and turns off entirely under prefers-reduced-motion.",
  },
};
let lang = "ru";
const t = (k) => (I18N[lang] && I18N[lang][k]) || I18N.ru[k] || k;

// manual-form extra fields per protocol
const EXTRA = {
  wisteria: [["pub", "Server public key (base64, optional)"], ["min_pad", "min_pad"], ["max_pad", "max_pad"],
             ["mimic", "mimic_tls (0/1)"], ["obf_mu", "mu_log"], ["obf_sigma", "sigma"]],
  ss: [["method", "cipher method"], ["password", "password"]],
  vmess: [["uuid", "UUID"], ["transport", "transport (tcp/ws/grpc)"], ["path", "path"], ["ws_host", "host (ws)"], ["tls", "tls (1/empty)"], ["sni", "sni"]],
  vless: [["uuid", "UUID"], ["security", "security (tls/reality/empty)"], ["sni", "sni"], ["transport", "transport"],
          ["path", "path"], ["public_key", "REALITY public_key"], ["short_id", "REALITY short_id"], ["flow", "flow"]],
  trojan: [["password", "password"], ["sni", "sni"], ["transport", "transport"], ["path", "path"]],
  hysteria2: [["password", "auth password"], ["sni", "sni"], ["insecure", "insecure (1/empty)"],
              ["obfs", "obfs (salamander)"], ["obfs_password", "obfs-password"]],
  tuic: [["uuid", "UUID"], ["password", "password"], ["congestion_control", "congestion_control"], ["sni", "sni"]],
};

// ---------- state ----------
const S = {
  tab: "vpn", setTab: "look", theme: "sunset", bg: "off", profiles: [], selected: "", lastLogSeq: 0,
  state: { state: "disconnected" }, uptimeTimer: null, extIP: "",
  subs: [], subBusy: {}, busy: false, // busy = a Connect is in flight
  version: "", upd: null, updBusy: false, updReady: false, coreUpd: {},
  wdiag: null, // the Diagnostics page data (wisteria profiles only)
};
const $ = (id) => document.getElementById(id);
const esc = (s) => String(s == null ? "" : s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const typeIcon = (tp) => ({ wisteria: I.lock }[tp] || I.globe);

function showToast(msg) {
  const el = $("toast");
  el.textContent = msg;
  el.classList.add("show");
  clearTimeout(el._timer);
  el._timer = setTimeout(() => el.classList.remove("show"), 2500);
}

// Styled replacement for window.confirm (the WebView's native dialog
// looks alien and says "wails.localhost"). Resolves true/false.
function askConfirm(msg, okLabel) {
  return new Promise((resolve) => {
    $("cf-msg").textContent = msg;
    $("cf-ok").textContent = okLabel || t("confirm.deleteBtn");
    $("modal-confirm").classList.add("show");
    const done = (v) => {
      $("modal-confirm").classList.remove("show");
      $("cf-ok").removeEventListener("click", onOk);
      $("cf-cancel").removeEventListener("click", onCancel);
      $("modal-confirm").removeEventListener("click", onOverlay);
      document.removeEventListener("keydown", onKey);
      resolve(v);
    };
    const onOk = () => done(true), onCancel = () => done(false);
    const onOverlay = (ev) => { if (ev.target.classList.contains("modal-overlay")) onCancel(); };
    const onKey = (ev) => { if (ev.key === "Escape") onCancel(); };
    $("cf-ok").addEventListener("click", onOk);
    $("cf-cancel").addEventListener("click", onCancel);
    $("modal-confirm").addEventListener("click", onOverlay);
    document.addEventListener("keydown", onKey);
  });
}

// ---------- navigation ----------
function switchTab(tab) {
  S.tab = tab;
  document.querySelectorAll(".sidebar-nav-item").forEach((el) => el.classList.toggle("active", el.dataset.tab === tab));
  renderContent();
  if (tab === "logs") pullLogs();
}
document.querySelectorAll(".sidebar-nav-item").forEach((b) => b.addEventListener("click", () => switchTab(b.dataset.tab)));

// ---------- rendering ----------
function renderContent() {
  const w = $("content-wrapper");
  if (S.tab === "vpn") w.innerHTML = renderVpn();
  else if (S.tab === "profiles") w.innerHTML = renderProfiles();
  else if (S.tab === "logs") w.innerHTML = renderLogs();
  else if (S.tab === "diag") { w.innerHTML = renderDiag(); pullWDiag(); }
  else if (S.tab === "settings") { w.innerHTML = renderSettings(); fillSettingsForm(); }
  wireContent();
  renderSidebarFooter();
  if (S.tab === "vpn") ensureExtIP();
}

function renderSidebarFooter() {
  const st = S.state;
  // Before the first connect the backend status carries no profile_id —
  // fall back to the user's selected (active) profile so the choice made
  // on the profiles tab is visible immediately, not only after Connect.
  const p = S.profiles.find((x) => x.id === (st.profile_id || S.selected));
  const rec = !!st.reconnecting;
  const dot = st.state === "connected" ? "on" : st.state === "connecting" || rec ? "mid" : "";
  const statusText = st.state === "connected" ? t("vpn.connected") : rec ? t("vpn.reconnecting") : st.state === "connecting" ? t("vpn.connecting") : t("vpn.unprotected");
  $("sidebar-footer").innerHTML = `
    <div class="sf-status"><div class="sf-dot ${dot}"></div> ${esc(statusText)}</div>
    <div class="sf-server">${esc(p ? p.name : (st.profile_name || "—"))}</div>
    <div class="sf-ip">${esc(p ? p.host + ":" + p.port : (st.socks_addr || ""))}</div>`;
}

function renderVpn() {
  const st = S.state;
  const conn = st.state === "connected";
  const connecting = st.state === "connecting";
  const rec = !!st.reconnecting; // session died; the shell is auto-retrying
  // Same as the sidebar footer: show the selected profile before the
  // backend status ever mentions one (i.e. before the first Connect).
  const p = S.profiles.find((x) => x.id === (st.profile_id || S.selected)) || null;
  const label = conn ? t("vpn.protected") : rec ? t("vpn.reconnecting") : connecting ? t("vpn.connecting") : t("vpn.unprotected");
  const btnText = conn || rec ? t("vpn.disconnect") : t("vpn.connect");
  const lat = p && p.latency_ms ? (p.latency_ms < 0 ? "n/a" : p.latency_ms + " ms") : "";
  return `
    <div class="vpn-screen">
      <div class="vpn-status"><div class="label">${esc(label)}</div></div>
      <div id="btn-wrap" style="position:relative; display:flex; justify-content:center;">
        <button class="connect-btn ${conn ? "connected" : ""}" id="btn-toggle" ${connecting && !rec ? "disabled" : ""}>
          <div class="ring-anim"></div>
          <div class="ring-anim d2"></div>
          ${I.power}
          <span class="connect-text">${esc(btnText)}</span>
        </button>
      </div>
      <div class="profile-card" id="card-profile">
        <div class="icon-box">${typeIcon(p ? p.type : "")}</div>
        <div class="srv-info">
          <div class="srv-name">${esc(p ? p.name : t("vpn.noProfile"))}</div>
          <div class="srv-ping">${esc(p ? p.host + ":" + p.port + (lat ? " · " + lat : "") : t("vpn.clickSelect"))}</div>
        </div>
        ${I.chev}
      </div>
      <div class="stats-row">
        <div class="stat-card"><div class="stat-label">${I.clock} ${esc(t("stat.uptime"))}</div><div class="stat-value" id="stat-uptime">${uptimeHTML(st.started_at) || "—"}</div></div>
        <div class="stat-card"><div class="stat-label">${I.globe} IP</div><div class="stat-value" style="font-size:13px; line-height:2.4;" id="stat-ip">${esc(S.extIP || "—")}</div></div>
        <div class="stat-card"><div class="stat-label">${I.globe} ${esc(t("stat.proxy"))}</div><div class="stat-value">${st.sys_proxy ? "ON" : "OFF"}</div></div>
      </div>
      ${st.tun ? `<div class="tun-note">${esc(t("vpn.tunOn"))}</div>` : ""}
      ${rec ? `<div class="rec-note"><span class="spin-ic">${I.refresh}</span><span>${esc(t("vpn.autoReconnect").replace("{n}", st.attempt || 1))}</span></div>` : ""}
      ${st.error && !rec ? `<div class="error-box">${esc(st.error)}</div>` : ""}
    </div>`;
}

function fmtSubTime(iso) {
  if (!iso) return t("subs.never");
  const d = new Date(iso);
  return d.toLocaleDateString() + " " + d.toLocaleTimeString();
}

// Collapse state of the profile groups ("sub:<id>" and "manual"), kept
// across restarts in localStorage.
S.collapsed = (() => {
  try { return JSON.parse(localStorage.getItem("wisteria-collapsed") || "{}") || {}; }
  catch (e) { return {}; }
})();
function toggleCollapsed(key) {
  S.collapsed[key] = !S.collapsed[key];
  try { localStorage.setItem("wisteria-collapsed", JSON.stringify(S.collapsed)); } catch (e) {}
  renderContent();
}

function profileCard(p) {
  const active = p.id === S.selected;
  const lat = p.latency_ms
    ? `<div class="sli-lat ${p.latency_ms > 0 ? "good" : ""}">${p.latency_ms < 0 ? "ping n/a" : "ping " + p.latency_ms + " ms"}</div>`
    : "";
  return `
    <div class="server-list-item ${active ? "active-server" : ""}" data-id="${p.id}">
      <div class="sli-icon">${typeIcon(p.type)}</div>
      <div class="sli-data">
        <div class="sli-name">${esc(p.name)}</div>
        <div class="sli-addr">${esc(p.host)}:${p.port}</div>
        ${lat}
      </div>
      <button class="sli-act pingb" title="ping" data-id="${p.id}">${I.ping}</button>
      <button class="sli-act del" title="delete" data-id="${p.id}">${I.trash}</button>
      <span class="sli-check" style="${active ? "" : "display:none"}">${I.check}</span>
    </div>`;
}

function profileGrid(items, emptyKey) {
  return `<div class="servers-grid">${items.length
    ? items.map(profileCard).join("")
    : `<div class="empty-note">${esc(t(emptyKey))}</div>`}</div>`;
}

// One collapsible group: a subscription with its profiles, or "my profiles".
function renderGroup(key, title, metaHtml, items, emptyKey, actionsHtml) {
  const collapsed = !!S.collapsed[key];
  return `
    <div class="sub-group">
      <div class="sub-head ${collapsed ? "closed" : ""}" data-key="${esc(key)}">
        <span class="sub-chev">${I.chev}</span>
        <div class="sub-info">
          <div class="sub-name">${esc(title)}<span class="sub-count">${items.length}</span></div>
          ${metaHtml}
        </div>
        ${actionsHtml || ""}
      </div>
      ${collapsed ? "" : profileGrid(items, emptyKey)}
    </div>`;
}

function renderProfiles() {
  // Profiles whose subscription no longer exists (e.g. a hand-edited
  // store) fall back to the manual group instead of vanishing.
  const manual = S.profiles.filter((p) => !p.sub_id || !S.subs.some((x) => x.id === p.sub_id));
  const groups = S.subs.map((s) => {
    const meta = s.error
      ? `<div class="sub-meta sub-err">${esc(s.error)}</div>`
      : `<div class="sub-meta">${esc(t("subs.updated"))}: ${esc(fmtSubTime(s.updated_at))}</div>`;
    const actions = `
      <button class="sli-act sub-refresh ${S.subBusy[s.id] ? "spin" : ""}" title="${esc(t("subs.refresh"))}" data-id="${s.id}">${I.refresh}</button>
      <button class="sli-act del" title="delete" data-id="${s.id}">${I.trash}</button>`;
    return renderGroup("sub:" + s.id, s.name, meta,
      S.profiles.filter((p) => p.sub_id === s.id), "subs.emptyGroup", actions);
  }).join("");
  return `
    <div class="servers-header">
      <h2>${esc(t("profiles.title"))}</h2>
      <div class="header-actions">
        <button class="ghost-btn" id="btn-clip" title="${esc(t("btn.clipTip"))}">${I.clipboard}<span>${esc(t("btn.clip"))}</span></button>
        <button class="add-btn" id="btn-add" title="${esc(t("add.title"))}">${I.plus}</button>
      </div>
    </div>
    ${groups}
    ${S.subs.length
      ? renderGroup("manual", t("profiles.manual"), "", manual, "profiles.empty", "")
      : profileGrid(manual, "profiles.empty")}`;
}

function renderLogs() {
  return `
    <div class="logs-header">
      <h2>${esc(t("logs.title"))}</h2>
      <div class="logs-tools">
        <label><input type="checkbox" id="log-follow" checked/> ${esc(t("logs.follow"))}</label>
        <button class="ghost-btn" id="btn-clear-logs">${esc(t("logs.clear"))}</button>
      </div>
    </div>
    <pre class="log-view" id="log-view"></pre>`;
}

// ---------- diagnostics (wisteria) ----------
// The wisteria core classifies its own connect loop: "cannot reach" = no
// network, "handshake refused" = blocked / stale key. We show the latest
// verdict as a banner and keep the raw event feed below.
function diagBanner(d) {
  const st = d.status.state;
  if (st === "connected" || d.last_kind === "up") return ["ok", "diag.bUp"];
  if (st === "rejected" || d.last_kind === "blocked") return ["bad", "diag.bBlocked"];
  if (st === "no_network" || d.last_kind === "nonet") return ["bad", "diag.bNonet"];
  if (st === "retry" || st === "connecting" || d.last_kind === "retry") return ["warn", "diag.bRetry"];
  return ["warn", "diag.bDown"];
}

function diagEvText(e) {
  if (lang !== "ru" && e.raw) return e.raw.replace(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2} /, "").trim();
  return e.text;
}

function diagInner(d) {
  if (!d.active) {
    return `
      <div class="diag-banner warn"><div class="diag-banner-ic">${I.lock}</div><div><b>${esc(t("diag.inactive"))}</b><p>${esc(t("diag.inactiveHint"))}</p></div></div>
      <button class="ghost-btn" id="btn-diag-logs">${esc(t("diag.openLogs"))}</button>`;
  }
  const [cls, key] = diagBanner(d);
  const ev = d.events || [], srv = d.servers || [];
  const attempts = ev.filter((e) => e.kind === "retry").length;
  const rtts = srv.map((s) => s.rtt_ms).filter((r) => r > 0);
  const best = rtts.length ? Math.min(...rtts) : 0;
  const okSrv = srv.filter((s) => s.reachable).length;
  const chips = `
    <div class="diag-chips">
      <div class="diag-chip"><span>${esc(t("diag.attempts"))}</span><b>${attempts}</b></div>
      <div class="diag-chip"><span>${esc(t("diag.bestRtt"))}</span><b>${best ? best + " ms" : "—"}</b></div>
      <div class="diag-chip"><span>${esc(t("diag.available"))}</span><b>${srv.length ? okSrv + "/" + srv.length : "—"}</b></div>
    </div>`;
  const table = srv.length ? `
    <div class="diag-card">
      <h3>${esc(t("diag.servers"))}</h3>
      <table class="diag-table">
        <thead><tr><th>${esc(t("diag.endpoint"))}</th><th>${esc(t("diag.rtt"))}</th><th>${esc(t("diag.reach"))}</th><th>${esc(t("diag.fails"))}</th></tr></thead>
        <tbody>${srv.map((s) => `
          <tr>
            <td class="mono">${esc(s.endpoint)}</td>
            <td>${s.rtt_ms > 0 ? s.rtt_ms + " ms" : "—"}</td>
            <td><span class="dg-dot ${s.reachable ? "on" : "off"}"></span>${s.reachable ? "✓" : "✕"}</td>
            <td>${s.failures}</td>
          </tr>`).join("")}</tbody>
      </table>
    </div>` : "";
  const feed = `
    <div class="diag-card">
      <h3>${esc(t("diag.events"))}</h3>
      ${ev.length ? `<div class="diag-events" id="diag-events">${ev.map((e) => `
        <div class="diag-ev"><span class="diag-ev-time">${esc(new Date(e.at).toLocaleTimeString())}</span><span class="dg-k dg-k-${esc(e.kind)}">${esc(e.kind)}</span><span class="diag-ev-text">${esc(diagEvText(e))}</span></div>`).join("")}</div>`
      : `<p class="diag-empty">${esc(t("diag.noEvents"))}</p>`}
    </div>`;
  return `
    <div class="diag-banner ${cls}"><div class="diag-banner-ic">${cls === "ok" ? I.check : cls === "bad" ? I.xmark : I.clock}</div><div><b>${esc(t(key))}</b></div></div>
    ${chips}${table}${feed}
    <button class="ghost-btn" id="btn-diag-logs">${esc(t("diag.openLogs"))}</button>`;
}

function renderDiag() {
  return `<div class="logs-header"><h2>${esc(t("diag.title"))}</h2></div><div id="diag-root">${S.wdiag ? diagInner(S.wdiag) : ""}</div>`;
}

let wdiagBusy = false;
async function pullWDiag() {
  if (wdiagBusy) return;
  wdiagBusy = true;
  try {
    const d = await api().GetWisteriaDiag();
    S.wdiag = d;
    const w = $("diag-root");
    if (!w || S.tab !== "diag") return;
    const ev = d.events || [];
    const sig = [d.active, d.status.state, d.last_kind, d.updated, ev.length, ev.length ? ev[ev.length - 1].at : ""].join("|");
    if (w._sig === sig) return;
    w._sig = sig;
    w.innerHTML = diagInner(d);
    const feedEl = $("diag-events");
    if (feedEl) feedEl.scrollTop = feedEl.scrollHeight;
  } catch (e) { /* diagnostics is best-effort */ }
  finally { wdiagBusy = false; }
}
setInterval(() => { if (S.tab === "diag") pullWDiag(); }, 2000);

function renderSettings() {
  const tabs = [
    ["look", "set.tabLook"],
    ["general", "set.tabGeneral"],
    ["conn", "set.tabConn"],
    ["routing", "set.tabRouting"],
    ["cores", "set.tabCores"],
  ];
  const bar = `<div class="set-tabs">${tabs.map(([id, k]) =>
    `<button class="set-tab ${S.setTab === id ? "active" : ""}" data-settab="${id}">${esc(t(k))}</button>`).join("")}</div>`;
  let body = "";
  if (S.setTab === "general") {
    body = `
    <div class="settings-section">
      <h3>${esc(t("sec.updates"))}</h3>
      <div class="set-grid">
        <label>${esc(t("set.updCheck"))}<span class="set-note">${esc(t("set.updNote"))}</span></label>
        <label class="switch"><input id="s-updcheck" type="checkbox"/><span class="slider"></span></label>
        <label>${esc(t("set.appVersion"))}</label>
        <div style="display:flex; gap:10px; align-items:center;">
          <span class="core-note" style="margin:0;">v${esc(S.version || "?")}</span>
          <button class="ghost-btn" id="btn-check-upd">${esc(t("set.checkUpd"))}</button>
        </div>
      </div>
      ${S.upd && S.upd.has_update ? `
      <div class="upd-banner">
        <span>${esc(t("set.updAvail"))}: v${esc(S.upd.latest)}</span>
        ${S.updReady
          ? `<button class="ghost-btn" id="btn-restart-app">${esc(t("set.updRestart"))}</button>`
          : `<button class="ghost-btn" id="btn-apply-upd" ${S.updBusy ? "disabled" : ""}>${esc(t("set.updApply"))}</button>`}
      </div>` : ""}
    </div>
    <div class="settings-section">
      <h3>${esc(t("sec.general"))}</h3>
      <div class="set-grid">
        <label>${esc(t("set.autostart"))}</label>
        <label class="switch"><input id="s-autostart" type="checkbox"/><span class="slider"></span></label>
        <label>${esc(t("set.startMin"))}<span class="set-note">${esc(t("set.startMinNote"))}</span></label>
        <label class="switch"><input id="s-startmin" type="checkbox"/><span class="slider"></span></label>
        <label>${esc(t("set.confirmExit"))}</label>
        <label class="switch"><input id="s-confirmexit" type="checkbox"/><span class="slider"></span></label>
        <label>${esc(t("set.checkIp"))}<span class="set-note">${esc(t("set.checkIpNote"))}</span></label>
        <label class="switch"><input id="s-checkip" type="checkbox"/><span class="slider"></span></label>
        <label>${esc(t("set.autoping"))}<span class="set-note">${esc(t("set.autopingNote"))}</span></label>
        <input id="s-autoping" type="number" min="0" max="3600"/>
        <label>${esc(t("set.autoconnect"))}</label>
        <select id="s-autoconnect"><option value="">—</option></select>
      </div>
      <div style="margin-top:10px;"><button class="ghost-btn" id="btn-checkip">${esc(t("set.checkIpNow"))}</button></div>
    </div>`;
  } else if (S.setTab === "conn") {
    body = `
    <div class="settings-section">
      <h3>${esc(t("sec.connection"))}</h3>
      <div class="set-grid">
        <label>${esc(t("set.socksPort"))}</label><input id="s-port" type="number" min="1024" max="65535"/>
        <label>${esc(t("set.httpPort"))}</label><input id="s-http-port" type="number" min="1024" max="65535"/>
        <label>${esc(t("set.bridge"))}</label>
        <label class="switch"><input id="s-bridge" type="checkbox"/><span class="slider"></span></label>
        <label>${esc(t("set.sysproxy"))}</label>
        <label class="switch"><input id="s-sysproxy" type="checkbox"/><span class="slider"></span></label>
        <label>${esc(t("set.listenAll"))}<span class="set-note">${esc(t("set.listenAllNote"))}</span></label>
        <label class="switch"><input id="s-listenall" type="checkbox"/><span class="slider"></span></label>
        <label>${esc(t("set.tun"))}<span class="set-note">${esc(t("set.tunNote"))}</span></label>
        <label class="switch"><input id="s-tun" type="checkbox"/><span class="slider"></span></label>
        <label>${esc(t("set.tunStack"))}</label>
        <select id="s-tunstack"><option value="mixed">mixed</option><option value="gvisor">gvisor</option><option value="system">system</option></select>
        <label>${esc(t("set.tunStrict"))}<span class="set-note">${esc(t("set.tunStrictNote"))}</span></label>
        <label class="switch"><input id="s-tunstrict" type="checkbox"/><span class="slider"></span></label>
      </div>
      <div style="display:flex; gap:10px; align-items:center; margin-top:10px;">
        <span class="set-note" style="margin:0;" id="tun-admin-note"></span>
        <button class="ghost-btn hidden" id="btn-admin">${esc(t("set.restartAdmin"))}</button>
      </div>
    </div>`;
  } else if (S.setTab === "routing") {
    body = `
    <div class="settings-section">
      <h3>${esc(t("sec.routing"))} <span class="sec-note">sing-box / xray</span></h3>
      <div class="set-grid">
        <label>${esc(t("set.bypassLan"))}</label>
        <label class="switch"><input id="s-bypasslan" type="checkbox"/><span class="slider"></span></label>
        <label>${esc(t("set.routeDirect"))}<span class="set-note">${esc(t("set.routeDirectNote"))}</span></label>
        <textarea id="s-route-direct" rows="4" placeholder="example.com"></textarea>
        <label>${esc(t("set.routeBlock"))}<span class="set-note">${esc(t("set.routeBlockNote"))}</span></label>
        <textarea id="s-route-block" rows="4" placeholder="ads.example.com"></textarea>
        <label>${esc(t("set.remoteDns"))}<span class="set-note">${esc(t("set.remoteDnsNote"))}</span></label>
        <input id="s-remotedns" type="text" placeholder="tcp://8.8.8.8"/>
        <label>${esc(t("set.logLevel"))}</label>
        <select id="s-loglevel">
          <option value="trace">trace</option><option value="debug">debug</option>
          <option value="info">info</option><option value="warn">warn</option>
          <option value="error">error</option>
        </select>
      </div>
    </div>`;
  } else if (S.setTab === "cores") {
    body = `
    <div class="settings-section">
      <h3>${esc(t("sec.engines"))}</h3>
      <div class="set-grid">
        <label>${esc(t("set.coreSS"))}</label>
        <select id="s-core-ss"><option value="sing-box">sing-box</option><option value="xray">xray-core</option></select>
        <label>${esc(t("set.coreVmess"))}</label>
        <select id="s-core-vmess"><option value="sing-box">sing-box</option><option value="xray">xray-core</option></select>
        <label>${esc(t("set.coreVless"))}</label>
        <select id="s-core-vless"><option value="sing-box">sing-box</option><option value="xray">xray-core</option></select>
        <label>${esc(t("set.coreTrojan"))}</label>
        <select id="s-core-trojan"><option value="sing-box">sing-box</option><option value="xray">xray-core</option></select>
      </div>
    </div>
    <div class="settings-section">
      <h3>${esc(t("sec.cores"))}</h3>
      <div class="set-grid">
        <label>${esc(t("set.wisteriaCore"))}</label><input id="s-wisteria" type="text"/>
        <label>${esc(t("set.singboxCore"))}</label><input id="s-singbox" type="text"/>
        <label>${esc(t("set.xrayCore"))}</label><input id="s-xray" type="text"/>
        <label>${esc(t("set.memLimit"))}<span class="set-note">${esc(t("set.memLimitNote"))}</span></label>
        <input id="s-memlimit" type="number" min="0" max="16384"/>
      </div>
      <div style="display:flex; gap:10px; margin-top:14px; align-items:center;">
        <button class="ghost-btn" id="btn-detect">${esc(t("set.detect"))}</button>
        <button class="ghost-btn" id="btn-upd-singbox" ${S.coreUpd["sing-box"] ? "disabled" : ""}>${esc(t("set.updCore"))} sing-box</button>
        <button class="ghost-btn" id="btn-upd-xray" ${S.coreUpd["xray"] ? "disabled" : ""}>${esc(t("set.updCore"))} xray</button>
      </div>
      <div class="core-note" id="core-note"></div>
    </div>`;
  } else {
    body = `
    <div class="settings-section">
      <h3>${esc(t("sec.appearance"))}</h3>
      <div class="theme-grid">${THEMES.map((th) => `
        <div class="theme-option ${S.theme === th.id ? "selected" : ""}" data-theme="${th.id}">
          <div class="theme-dot" style="background:${th.color}"></div>
          <div class="theme-name">${esc(th.name)}</div>
        </div>`).join("")}
      </div>
      <div class="set-grid" style="margin-top:12px;">
        <label>${esc(t("set.lang"))}</label>
        <select id="s-lang"><option value="ru">Русский</option><option value="en">English</option></select>
      </div>
    </div>
    <div class="settings-section">
      <h3>${esc(t("set.bgTitle"))}</h3>
      <div class="bg-grid">${Object.keys(BGPRESETS).map((id) => `
        <div class="bg-option ${S.bg === id ? "selected" : ""}" data-bg="${id}">
          <div class="bg-swatch" style="background:${BGPRESETS[id].swatch}"></div>
          <div class="bg-name">${esc(t(BGPRESETS[id].name))}</div>
        </div>`).join("")}
      </div>
      <div class="bg-note">${esc(t("set.bgNote"))}</div>
    </div>`;
  }
  return `<div class="settings-title">${esc(t("settings.title"))}</div>${bar}${body}
    <div style="margin:6px 0 30px;">
      <button class="submit-btn save-btn" id="btn-save-settings">${esc(t("set.save"))}</button>
    </div>`;
}

// Optimistically show "connecting to X" the moment a switch starts, so the
// wait is visible; backend state events overwrite this as they arrive.
function markConnecting(id) {
  const p = S.profiles.find((x) => x.id === id);
  if (!p) return;
  renderState({ ...S.state, state: "connecting", reconnecting: false, attempt: 0, profile_id: p.id, profile_name: p.name, error: "" });
}

// Reject after ms so a wedged backend call can never lock the busy guard
// forever (the promise itself may still settle later; state events heal the UI).
function withTimeout(p, ms, msg) {
  return Promise.race([
    p,
    new Promise((_, rej) => setTimeout(() => rej(new Error(msg)), ms)),
  ]);
}

async function updateCore(name) {
  if (S.coreUpd[name]) return;
  S.coreUpd[name] = true;
  renderContent();
  try {
    const info = await api().CheckCoreUpdate(name);
    if (!info.has_update) {
      showToast(name + " v" + (info.current || "?") + " — " + t("toast.updNone"));
    } else {
      const ver = await api().UpdateCore(name);
      showToast("✓ " + name + " → v" + ver);
    }
  } catch (e) { showToast("✕ " + ((e && e.message) || e)); }
  S.coreUpd[name] = false;
  renderContent();
}

// ---------- per-render event wiring ----------
function wireContent() {
  const on = (id, ev, fn) => { const el = $(id); if (el) el.addEventListener(ev, fn); };

  on("btn-toggle", "click", async () => {
    if (S.busy) { showToast(t("toast.busy")); return; }
    try {
      // "connected" and the auto-reconnecting state both stop the engine:
      // Disconnect also cancels the reconnect chain (user intent wins).
      if (S.state.state === "connected" || S.state.reconnecting) api().Disconnect();
      else {
        if (!S.selected) { openModal(); return; }
        S.busy = true;
        markConnecting(S.selected);
        await withTimeout(api().Connect(S.selected), 45000, t("toast.stuck"));
      }
    } catch (e) {
      showToast((e && e.message) || e);
      // markConnecting drew a local "connecting" screen; a fast backend
      // failure returns with no state event of its own — re-pull the real
      // state so the UI never stays wedged until a restart.
      try { renderState(await api().GetState()); } catch (_) { /* keep local view */ }
    }
    finally { S.busy = false; }
  });
  on("card-profile", "click", () => switchTab("profiles"));

  on("btn-add", "click", openModal);
  document.querySelectorAll(".server-list-item").forEach((row) => {
    row.addEventListener("click", async (ev) => {
      if (ev.target.closest(".sli-act")) return;
      if (S.busy) { showToast(t("toast.busy")); return; }
      S.selected = row.dataset.id;
      await api().SetActive(row.dataset.id);
      // Momentary switching: clicking a server while connected moves the
      // tunnel to it (the backend stops the old session first). The busy
      // flag keeps a second click / double-click from starting a parallel
      // session while the switch is still running.
      const wasConnected = S.state.state === "connected";
      renderContent();
      if (wasConnected) {
        S.busy = true;
        markConnecting(row.dataset.id);
        try { await withTimeout(api().Connect(row.dataset.id), 45000, t("toast.stuck")); }
        catch (e) {
          showToast((e && e.message) || e);
          try { renderState(await api().GetState()); } catch (_) { /* keep local view */ }
        }
        finally { S.busy = false; }
      }
    });
    row.addEventListener("dblclick", async (ev) => {
      if (ev.target.closest(".sli-act")) return;
      S.selected = row.dataset.id;
      await api().SetActive(row.dataset.id);
      switchTab("vpn"); // the click handler above already connects
    });
  });
  document.querySelectorAll(".sli-act.pingb").forEach((b) => b.addEventListener("click", async () => {
    await api().PingProfile(b.dataset.id);
    await loadProfiles();
  }));
  document.querySelectorAll(".sli-act.del").forEach((b) => b.addEventListener("click", async (ev) => {
    ev.stopPropagation();
    if (!(await askConfirm(t("confirm.delete")))) return;
    try { await api().DeleteProfile(b.dataset.id); await loadProfiles(); }
    catch (e) { showToast((e && e.message) || e); }
  }));

  // subscriptions are added from the clipboard ("From clipboard");
  // AddSubscription remains as an API for future import paths.
  // One-button import: the clipboard may hold subscription URLs, share
  // links or both — the backend routes every line.
  on("btn-clip", "click", async () => {
    try {
      const r = await api().ImportFromClipboard();
      await loadSubs();
      await loadProfiles();
      const parts = [];
      if (r.subscriptions) parts.push(t("toast.clipSubs").replace("{n}", r.subscriptions));
      if (r.added && r.added.length) parts.push(t("toast.clipProfiles").replace("{n}", r.added.length));
      showToast(parts.length ? parts.join(" · ") : t("toast.clipNone"));
      if (r.errors && r.errors.length) {
        const fe = r.errors[0];
        showToast((fe.line ? fe.line + ": " : "") + fe.error);
      }
    } catch (e) { showToast((e && e.message) || e); }
  });
  // Collapse/expand profile groups; state persists in localStorage.
  document.querySelectorAll(".sub-head").forEach((h) => h.addEventListener("click", (ev) => {
    if (ev.target.closest(".sli-act")) return;
    toggleCollapsed(h.dataset.key);
  }));
  document.querySelectorAll(".sub-refresh").forEach((b) => b.addEventListener("click", async (ev) => {
    ev.stopPropagation();
    await refreshSub(b.dataset.id);
  }));
  document.querySelectorAll(".sub-head .sli-act.del").forEach((b) => b.addEventListener("click", async (ev) => {
    ev.stopPropagation();
    if (!(await askConfirm(t("subs.confirmDelete")))) return;
    try {
      await api().DeleteSubscription(b.dataset.id);
      await loadSubs();
      await loadProfiles();
    } catch (e) { showToast((e && e.message) || e); }
  }));

  on("btn-clear-logs", "click", () => {
    // Keep S.lastLogSeq: resetting it would make the next pull refetch the
    // whole ring and re-add the history that was just cleared.
    $("log-view").textContent = "";
  });

  document.querySelectorAll(".theme-option").forEach((opt) => opt.addEventListener("click", async () => {
    S.theme = opt.dataset.theme;
    applyTheme();
    document.querySelectorAll(".theme-option").forEach((o) => o.classList.toggle("selected", o === opt));
    await saveSettings();
  }));
  document.querySelectorAll(".bg-option").forEach((opt) => opt.addEventListener("click", async () => {
    applyBackground(opt.dataset.bg);
    document.querySelectorAll(".bg-option").forEach((o) => o.classList.toggle("selected", o === opt));
    await saveSettings();
  }));
  on("btn-checkip", "click", async () => {
    try {
      const ip = await api().GetExternalIP();
      S.extIP = ip;
      const el = $("stat-ip");
      if (el) el.textContent = ip;
      showToast("IP: " + ip);
    } catch (e) {
      showToast("✕ " + t("toast.ipFail") + ": " + ((e && e.message) || e));
    }
  });
  document.querySelectorAll("[data-i18n]").forEach(applyI18nEl);

  on("btn-detect", "click", async () => {
    const info = await api().DetectCores();
    const fmt = (f, p) => (f ? "✓ " + p : "✕ not found");
    $("core-note").innerHTML =
      `wisteria-client: ${esc(fmt(info.wisteria_found, info.wisteria_path))}<br>` +
      `sing-box: ${esc(fmt(info.singbox_found, info.singbox_path))}<br>` +
      `xray: ${esc(fmt(info.xray_found, info.xray_path))}`;
  });
  on("btn-admin", "click", async () => {
    try { await api().RestartAsAdmin(); }
    catch (e) { showToast((e && e.message) || e); }
  });
  if (api().IsElevated) {
    api().IsElevated().then((el) => {
      const note = $("tun-admin-note");
      const btn = $("btn-admin");
      if (!note || !btn) return;
      if (!el) { note.textContent = t("set.notAdmin"); btn.classList.remove("hidden"); }
    });
  }
  on("btn-check-upd", "click", async () => {
    try {
      const info = await api().CheckAppUpdate();
      S.upd = info;
      S.updReady = false;
      if (!info.has_update) showToast("v" + info.current + " — " + t("toast.updNone"));
      renderContent();
    } catch (e) { showToast((e && e.message) || e); }
  });
  on("btn-apply-upd", "click", async () => {
    if (S.updBusy) return;
    S.updBusy = true;
    renderContent();
    try {
      await api().ApplyAppUpdate();
      S.updReady = true;
      showToast(t("toast.updReady"));
    } catch (e) { showToast("✕ " + ((e && e.message) || e)); }
    S.updBusy = false;
    renderContent();
  });
  on("btn-restart-app", "click", async () => {
    try { await api().RestartApp(); }
    catch (e) { showToast((e && e.message) || e); }
  });
  on("btn-upd-singbox", "click", () => updateCore("sing-box"));
  on("btn-upd-xray", "click", () => updateCore("xray"));
  // Cores tab: when a third-party core is not installed at all, the
  // upstream button is a first download, not an update — say so on the
  // button ("Скачать xray") instead of the misleading "Обновить".
  if ($("btn-upd-singbox") || $("btn-upd-xray")) {
    api().DetectCores().then((info) => {
      if (!info) return;
      const b1 = $("btn-upd-singbox"), b2 = $("btn-upd-xray");
      if (b1 && !info.singbox_found) b1.textContent = t("set.dlCore") + " sing-box";
      if (b2 && !info.xray_found) b2.textContent = t("set.dlCore") + " xray";
    }).catch(() => { /* detection is cosmetic here */ });
  }
  document.querySelectorAll(".set-tab").forEach((b) => b.addEventListener("click", () => {
    S.setTab = b.dataset.settab;
    renderContent();
  }));
  on("btn-save-settings", "click", saveSettings);
}

function applyI18nEl(el) {
  const k = el.dataset.i18n;
  if (!k) return;
  if (el.dataset.ph !== undefined || el.tagName === "TEXTAREA" && I18N[lang][k + "Ph"]) { /* handled below */ }
  el.textContent = t(k);
}

// ---------- settings ----------
async function loadSettings() {
  const s = await api().GetSettings();
  S.cfg = s;
  S.theme = s.theme || "lilac";
  S.bg = s.background || "off";
  lang = s.language || "ru";
  applyTheme();
  applyBackground(S.bg);
  // Values are filled when the settings view renders.
  if (S.tab === "settings") { renderContent(); fillSettingsForm(s); }
  return s;
}

async function fillSettingsForm(s) {
  if (!s) s = await api().GetSettings();
  const setv = (id, v) => { const el = $(id); if (el) el.value = v; };
  const setc = (id, v) => { const el = $(id); if (el) el.checked = !!v; };
  document.documentElement.lang = lang;
  setv("s-lang", lang);
  setv("s-port", s.local_port || 2080);
  setv("s-http-port", s.http_port || 2081);
  setc("s-sysproxy", s.sys_proxy);
  setc("s-bridge", s.bridge_enabled);
  setv("s-wisteria", s.wisteria_core || "");
  setv("s-singbox", s.singbox_core || "");
  setc("s-updcheck", !s.no_update_check);
  setc("s-autostart", await api().GetAutostart());
  setc("s-startmin", s.start_minimized);
  setc("s-confirmexit", s.confirm_exit);
  setc("s-checkip", s.check_ext_ip);
  setv("s-autoping", s.auto_ping_sec || 0);
  setc("s-listenall", s.listen_all);
  setc("s-bypasslan", s.bypass_lan);
  setv("s-remotedns", s.remote_dns || "");
  setv("s-loglevel", s.log_level || "warn");
  setv("s-memlimit", s.mem_limit_mb || 0);
  setc("s-tun", s.tun_enabled);
  setv("s-tunstack", s.tun_stack || "mixed");
  setc("s-tunstrict", s.tun_strict_route);
  setv("s-route-direct", s.route_direct || "");
  setv("s-route-block", s.route_block || "");
  setv("s-core-ss", s.core_ss || "sing-box");
  setv("s-core-vmess", s.core_vmess || "sing-box");
  setv("s-core-vless", s.core_vless || "sing-box");
  setv("s-core-trojan", s.core_trojan || "sing-box");
  setv("s-xray", s.xray_core || "");
  const sel = $("s-autoconnect");
  if (sel) {
    sel.innerHTML = `<option value="">—</option>` + S.profiles.map((p) =>
      `<option value="${p.id}">${esc(p.name)} (${esc(p.type)})</option>`).join("");
    sel.value = s.auto_connect || "";
  }
}

async function saveSettings() {
  const cur = await api().GetSettings();
  const g = (id) => { const el = $(id); return el ? el : null; };
  const num = (id, def) => { const v = parseInt($(id).value, 10); return Number.isFinite(v) ? v : def; };
  const next = {
    language: lang,
    theme: S.theme,
    background: S.bg,
    local_port: g("s-port") ? num("s-port", 2080) : cur.local_port,
    http_port: g("s-http-port") ? num("s-http-port", 2081) : cur.http_port,
    sys_proxy: g("s-sysproxy") ? $("s-sysproxy").checked : cur.sys_proxy,
    bridge_enabled: g("s-bridge") ? $("s-bridge").checked : cur.bridge_enabled,
    auto_connect: g("s-autoconnect") ? $("s-autoconnect").value : cur.auto_connect,
    wisteria_core: g("s-wisteria") ? $("s-wisteria").value.trim() : cur.wisteria_core,
    singbox_core: g("s-singbox") ? $("s-singbox").value.trim() : cur.singbox_core,
    start_minimized: g("s-startmin") ? $("s-startmin").checked : cur.start_minimized,
    confirm_exit: g("s-confirmexit") ? $("s-confirmexit").checked : cur.confirm_exit,
    check_ext_ip: g("s-checkip") ? $("s-checkip").checked : cur.check_ext_ip,
    no_update_check: g("s-updcheck") ? !$("s-updcheck").checked : cur.no_update_check,
    github_token: g("s-ghtoken") ? $("s-ghtoken").value.trim() : cur.github_token,
    auto_ping_sec: g("s-autoping") ? Math.max(0, num("s-autoping", 0)) : cur.auto_ping_sec,
    listen_all: g("s-listenall") ? $("s-listenall").checked : cur.listen_all,
    bypass_lan: g("s-bypasslan") ? $("s-bypasslan").checked : cur.bypass_lan,
    remote_dns: g("s-remotedns") ? $("s-remotedns").value.trim() : cur.remote_dns,
    log_level: g("s-loglevel") ? $("s-loglevel").value : cur.log_level,
    mem_limit_mb: g("s-memlimit") ? Math.max(0, num("s-memlimit", 0)) : cur.mem_limit_mb,
    tun_enabled: g("s-tun") ? $("s-tun").checked : cur.tun_enabled,
    tun_stack: g("s-tunstack") ? $("s-tunstack").value : cur.tun_stack,
    tun_strict_route: g("s-tunstrict") ? $("s-tunstrict").checked : cur.tun_strict_route,
    route_direct: g("s-route-direct") ? $("s-route-direct").value.trim() : cur.route_direct,
    route_block: g("s-route-block") ? $("s-route-block").value.trim() : cur.route_block,
    mp_core: cur.mp_core, // legacy default engine; per-protocol picks are below
    core_ss: g("s-core-ss") ? $("s-core-ss").value : cur.core_ss,
    core_vmess: g("s-core-vmess") ? $("s-core-vmess").value : cur.core_vmess,
    core_vless: g("s-core-vless") ? $("s-core-vless").value : cur.core_vless,
    core_trojan: g("s-core-trojan") ? $("s-core-trojan").value : cur.core_trojan,
    xray_core: g("s-xray") ? $("s-xray").value.trim() : cur.xray_core,
  };
  await api().SaveSettings(next);
  const as = g("s-autostart");
  if (as && as.checked !== (await api().GetAutostart())) await api().SetAutostart(as.checked);
  showToast(t("toast.saved"));
}

function applyTheme() {
  $("app").className = "theme-" + S.theme;
}

// ---------- profiles ----------
async function loadProfiles() {
  S.profiles = (await api().ListProfiles()) || [];
  const st = await api().GetState();
  if (st.profile_id) S.selected = st.profile_id;
  else S.selected = await api().GetActiveID();
  renderContent();
}

// ---------- subscriptions ----------
async function loadSubs() {
  S.subs = (await api().ListSubscriptions()) || [];
  if (S.tab === "profiles") renderContent();
}

async function refreshSub(id) {
  if (S.subBusy[id]) return;
  S.subBusy[id] = true;
  if (S.tab === "profiles") renderContent(); // spin the refresh icon
  try {
    const sub = await api().RefreshSubscription(id);
    if (sub && sub.error) showToast(sub.error);
  } catch (e) {
    showToast((e && e.message) || e); // the error is also stored on the sub row
  }
  S.subBusy[id] = false;
  await loadSubs();
  await loadProfiles();
}

// ---------- logs ----------
async function pullLogs() {
  const lines = (await api().GetLogs(S.lastLogSeq)) || [];
  if (!lines.length) return;
  const view = $("log-view");
  if (!view) return;
  const follow = $("log-follow") ? $("log-follow").checked : true;
  const frag = document.createDocumentFragment();
  lines.forEach((l) => {
    const div = document.createElement("span");
    div.className = "l-" + l.tag;
    div.textContent = `${new Date(l.at).toLocaleTimeString()} [${l.tag}] ${l.text}\n`;
    frag.appendChild(div);
  });
  view.appendChild(frag);
  while (view.childNodes.length > 4000) view.removeChild(view.firstChild);
  if (follow) view.scrollTop = view.scrollHeight;
  S.lastLogSeq = lines[lines.length - 1].seq;
}

// ---------- uptime ----------
// Uptime is re-painted every second straight from S.state: no start/stop
// timer pairing to desync, survives any re-render of the VPN screen.
function uptimeHTML(iso) {
  if (!iso || String(iso).startsWith("0001")) return null;
  const s = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 1000));
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), ss = s % 60;
  return (h ? h + "<span class='stat-unit'>h </span>" : "") +
    m + "<span class='stat-unit'>m </span>" + ss + "<span class='stat-unit'>s</span>";
}
setInterval(() => {
  if (S.tab !== "vpn" || S.state.state !== "connected") return;
  const el = $("stat-uptime");
  if (!el) return;
  const html = uptimeHTML(S.state.started_at);
  if (html) el.innerHTML = html;
}, 1000);

// ---------- state self-heal ----------
// The state is pushed from the Go side as events; one lost event (the
// message pump wedge noted in app.go, a webview reload, an emit before
// EventsOn registration) used to leave the UI frozen on a stale status
// — up to a restart. The status now mirrors the core within 5 s at worst:
// poll the real status and reconcile whenever it differs.
function stateSig(st) {
  return [st.state, st.reconnecting ? 1 : 0, st.attempt || 0, st.profile_id || "",
    st.error || "", st.tun ? 1 : 0, st.sys_proxy ? 1 : 0, st.pid || 0, st.started_at || ""].join("|");
}
setInterval(async () => {
  if (S.busy || !api()) return; // an in-flight Connect owns the view
  try {
    const st = await api().GetState();
    if (stateSig(st) !== stateSig(S.state)) renderState(st);
  } catch (e) { /* backend not ready */ }
}, 5000);

// ---------- ambient background (canvas presets) ----------
// Data-driven preset list: adding a preset means adding a row here (and a
// settings i18n label), no engine code. Rules: rAF only, ~30 fps cap,
// paused on window blur / hidden tab, static frame under
// prefers-reduced-motion, off by default. No GIFs, no external assets.
const BGPRESETS = {
  off: {
    name: "bg.off", kind: "none",
    swatch: "repeating-linear-gradient(135deg, #14141a 0 8px, #101014 8px 16px)",
  },
  lilac: {
    name: "bg.lilac", kind: "bokeh", scale: 0.5,
    swatch: "radial-gradient(circle at 30% 40%, rgba(167,139,250,.55), transparent 60%), radial-gradient(circle at 70% 65%, rgba(139,92,246,.45), transparent 55%), #14101f",
    count: 14, sizes: [50, 220], speed: 10, alpha: 0.16,
    colors: [[167, 139, 250], [196, 181, 253], [139, 92, 246]],
  },
  matrix: {
    name: "bg.matrix", kind: "rain", scale: 1.0,
    swatch: "repeating-linear-gradient(90deg, rgba(0,255,65,.20) 0 2px, transparent 2px 10px), #031006",
    color: "0,255,65", alpha: 0.10, fontSize: 14, speed: 55, active: 0.35,
    glyphs: "アカサタナハマヤラワ0123456789#$%&*+=",
  },
  gradient: {
    name: "bg.gradient", kind: "gradient", scale: 0.5,
    swatch: "linear-gradient(120deg, #4c1d95, #1e3a8a, #0e7490)",
    colors: [[88, 40, 170], [16, 110, 140], [40, 34, 96]], alpha: 0.10, speed: 0.06,
  },
};

const BG = {
  cv: null, cx: null, raf: 0, id: "off", sim: null,
  w: 0, h: 0, dpr: 1, scale: 1, last: 0, running: false, fpsMs: 1000 / 30,
};

const bgMotionOK = () => !(window.matchMedia && matchMedia("(prefers-reduced-motion: reduce)").matches);

function bgCanvas() {
  if (!BG.cv) {
    BG.cv = $("bgfx");
    BG.cx = BG.cv ? BG.cv.getContext("2d") : null;
  }
  return BG.cv && BG.cx;
}

function bgRunning() {
  // The loop runs only while: a preset is on, the page is visible, the
  // window has focus, and the OS does not ask for reduced motion.
  return BG.id !== "off" && !document.hidden && document.hasFocus() && bgMotionOK();
}

function bgColumn(p, fs, x) {
  return {
    x,
    y: -Math.random() * BG.h,
    speed: p.speed * (0.45 + Math.random() * 0.8),
    seed: Math.floor(Math.random() * 1000),
    idle: Math.random() >= p.active, // quiet matrix: only a share of columns drips
  };
}

function bgInit() {
  const p = BGPRESETS[BG.id];
  BG.sim = null;
  if (!p || p.kind === "none") return;
  if (p.kind === "bokeh") {
    const parts = [];
    for (let i = 0; i < p.count; i++) {
      parts.push({
        x: Math.random() * BG.w, y: Math.random() * BG.h,
        vx: (Math.random() - 0.5) * p.speed * BG.dpr,
        vy: (Math.random() - 0.5) * p.speed * BG.dpr,
        r: (p.sizes[0] + Math.random() * (p.sizes[1] - p.sizes[0])) * BG.dpr,
        c: p.colors[i % p.colors.length],
        ph: Math.random() * Math.PI * 2,
      });
    }
    BG.sim = { parts };
  } else if (p.kind === "rain") {
    const fs = p.fontSize * BG.dpr;
    const pitch = fs * 1.4;
    const n = Math.ceil(BG.w / pitch);
    const cols = [];
    for (let i = 0; i < n; i++) cols.push(bgColumn(p, fs, i * pitch));
    BG.sim = { fs, cols };
  } else if (p.kind === "gradient") {
    BG.sim = { t: Math.random() * 100 };
  }
}

function bgResize() {
  if (!bgCanvas()) return;
  const p = BGPRESETS[BG.id];
  const r = BG.cv.getBoundingClientRect();
  BG.dpr = Math.min(window.devicePixelRatio || 1, 1.5);
  BG.scale = (p && p.scale) || 1;
  const k = BG.dpr * BG.scale; // soft presets render half-size: 4x less fill
  BG.w = Math.max(1, Math.round(r.width * k));
  BG.h = Math.max(1, Math.round(r.height * k));
  BG.cv.width = BG.w;
  BG.cv.height = BG.h;
  bgInit();
  bgDraw(0); // static frame: content shows even while the loop is paused
}

function bgDraw(dt) {
  const p = BGPRESETS[BG.id];
  if (!p || p.kind === "none" || !BG.sim || !BG.cx) return;
  const cx = BG.cx;
  if (p.kind === "bokeh") {
    cx.clearRect(0, 0, BG.w, BG.h);
    cx.globalCompositeOperation = "lighter";
    for (const b of BG.sim.parts) {
      b.x += b.vx * dt; b.y += b.vy * dt; b.ph += dt * 0.4;
      if (b.x < -b.r) b.x = BG.w + b.r; else if (b.x > BG.w + b.r) b.x = -b.r;
      if (b.y < -b.r) b.y = BG.h + b.r; else if (b.y > BG.h + b.r) b.y = -b.r;
      const a = p.alpha * (0.6 + 0.4 * Math.sin(b.ph));
      const g = cx.createRadialGradient(b.x, b.y, 0, b.x, b.y, b.r);
      g.addColorStop(0, "rgba(" + b.c[0] + "," + b.c[1] + "," + b.c[2] + "," + a.toFixed(3) + ")");
      g.addColorStop(1, "rgba(" + b.c[0] + "," + b.c[1] + "," + b.c[2] + ",0)");
      cx.fillStyle = g;
      cx.fillRect(b.x - b.r, b.y - b.r, b.r * 2, b.r * 2);
    }
    cx.globalCompositeOperation = "source-over";
  } else if (p.kind === "rain") {
    cx.fillStyle = "rgba(2,12,5,0.07)"; // trail fade
    cx.fillRect(0, 0, BG.w, BG.h);
    cx.font = BG.sim.fs + "px 'JetBrains Mono', ui-monospace, 'Cascadia Mono', Consolas, monospace";
    for (const col of BG.sim.cols) {
      if (col.idle) continue;
      col.y += col.speed * dt * BG.dpr;
      if (col.y > BG.h + BG.sim.fs * 2) { Object.assign(col, bgColumn(p, BG.sim.fs, col.x)); continue; }
      const gy = Math.floor(col.y / BG.sim.fs);
      const ch = (k) => p.glyphs[((gy - k) * 7 + col.seed + 1000) % p.glyphs.length];
      cx.fillStyle = "rgba(" + p.color + "," + p.alpha + ")"; // soft tail
      cx.fillText(ch(2), col.x, (gy - 2) * BG.sim.fs);
      cx.fillText(ch(1), col.x, (gy - 1) * BG.sim.fs);
      cx.fillStyle = "rgba(" + p.color + "," + (p.alpha * 2.4).toFixed(3) + ")"; // bright head
      cx.fillText(ch(0), col.x, gy * BG.sim.fs);
    }
  } else if (p.kind === "gradient") {
    BG.sim.t += dt * p.speed;
    const t = BG.sim.t;
    cx.globalCompositeOperation = "source-over";
    cx.fillStyle = "rgba(8,7,12,0.30)"; // base: keeps the theme dark under the blobs
    cx.fillRect(0, 0, BG.w, BG.h);
    cx.globalCompositeOperation = "lighter";
    const R = Math.max(BG.w, BG.h) * 0.6;
    for (let i = 0; i < p.colors.length; i++) {
      const ph = (i / p.colors.length) * Math.PI * 2;
      const x = BG.w * (0.5 + 0.34 * Math.sin(t * 0.7 + ph));
      const y = BG.h * (0.5 + 0.30 * Math.cos(t * 0.55 + ph * 1.3));
      const c = p.colors[i];
      const g = cx.createRadialGradient(x, y, 0, x, y, R);
      g.addColorStop(0, "rgba(" + c[0] + "," + c[1] + "," + c[2] + "," + p.alpha + ")");
      g.addColorStop(1, "rgba(" + c[0] + "," + c[1] + "," + c[2] + ",0)");
      cx.fillStyle = g;
      cx.fillRect(0, 0, BG.w, BG.h);
    }
    cx.globalCompositeOperation = "source-over";
  }
}

// rAF with an explicit ~30 fps ceiling: on 60/120 Hz displays every second
// (third) vsync frame is dropped, which halves the paint cost.
function bgLoop(ts) {
  if (!BG.running) return;
  BG.raf = requestAnimationFrame(bgLoop);
  const dtms = ts - BG.last;
  if (dtms < BG.fpsMs) return;
  BG.last = ts;
  bgDraw(Math.min(dtms / 1000, 0.12));
}

function bgStartLoop() {
  if (BG.running) return;
  BG.running = true;
  BG.last = performance.now();
  BG.raf = requestAnimationFrame(bgLoop);
}

function bgStopLoop() {
  BG.running = false;
  if (BG.raf) cancelAnimationFrame(BG.raf);
  BG.raf = 0;
}

function bgSync() {
  if (bgRunning()) bgStartLoop();
  else bgStopLoop(); // the last static frame stays visible: no flicker on blur
}

function applyBackground(id) {
  if (!BGPRESETS[id]) id = "off";
  S.bg = id;
  BG.id = id;
  if (!bgCanvas()) return;
  if (id === "off") {
    bgStopLoop();
    BG.cx.clearRect(0, 0, BG.w, BG.h);
    BG.cv.classList.add("hidden");
    return;
  }
  BG.cv.classList.remove("hidden");
  bgResize();
  bgSync();
}

document.addEventListener("visibilitychange", bgSync);
window.addEventListener("focus", bgSync);
window.addEventListener("blur", bgSync);
window.addEventListener("resize", () => { if (BG.id !== "off") bgResize(); });

// Self-heal the external IP: the push event can be lost when the backend
// finishes the check before the frontend has registered its listeners
// (auto-connect at startup). If we are connected and the card is empty,
// pull it once through the tunnel.
let extIPBusy = false;
async function ensureExtIP() {
  if (extIPBusy || S.extIP || S.state.state !== "connected") return;
  if (S.cfg && S.cfg.check_ext_ip === false) return; // respect the setting, like the push event does
  extIPBusy = true;
  try {
    const ip = await api().GetExternalIP();
    if (ip) {
      S.extIP = ip;
      const el = $("stat-ip");
      if (el) el.textContent = ip;
    }
  } catch (e) { /* the card stays "—" until the next push */ }
  extIPBusy = false;
}

function renderState(st) {
  const prev = S.state;
  S.state = st || S.state;
  if (S.tab !== "vpn") { renderSidebarFooter(); return; }
  // Rebuild only on structural changes (profile switch, error box, TUN
  // note, proxy toggle). Same-profile transitions (idle → connecting →
  // connected) patch the DOM in place: an innerHTML swap would recreate
  // the button node and restart its CSS animation from frame zero, which
  // reads as the button animation stuttering on every state event.
  const structural = !prev ||
    prev.profile_id !== S.state.profile_id ||
    !!prev.tun !== !!S.state.tun ||
    (prev.error || "") !== (S.state.error || "") ||
    !!prev.sys_proxy !== !!S.state.sys_proxy ||
    !!prev.reconnecting !== !!S.state.reconnecting ||
    (prev.attempt || 0) !== (S.state.attempt || 0);
  if (structural) { renderContent(); return; }
  patchVpnState();
  renderSidebarFooter();
}

function patchVpnState() {
  const st = S.state;
  const conn = st.state === "connected";
  const connecting = st.state === "connecting";
  const rec = !!st.reconnecting;
  const labelEl = document.querySelector(".vpn-status .label");
  if (labelEl) labelEl.textContent = conn ? t("vpn.protected") : rec ? t("vpn.reconnecting") : connecting ? t("vpn.connecting") : t("vpn.unprotected");
  const btn = $("btn-toggle");
  if (btn) {
    btn.classList.toggle("connected", conn);
    btn.disabled = connecting && !rec;
    const txt = btn.querySelector(".connect-text");
    if (txt) txt.textContent = conn || rec ? t("vpn.disconnect") : t("vpn.connect");
  }
}

// ---------- modal ----------
function openModal() {
  $("modal-add").classList.add("show");
  $("import-area").value = "";
  $("import-errors").innerHTML = "";
  $("toml-status").innerHTML = "";
}
function closeModal() { $("modal-add").classList.remove("show"); }
window.UI = { closeModal };

document.querySelectorAll(".modal-tab").forEach((tb) => tb.addEventListener("click", () => {
  document.querySelectorAll(".modal-tab").forEach((x) => x.classList.toggle("active", x === tb));
  document.querySelectorAll(".modal-body").forEach((b) => b.classList.toggle("hidden", b.dataset.mbody !== tb.dataset.mtab));
}));
document.querySelector(".modal-overlay").addEventListener("click", (ev) => {
  if (ev.target.classList.contains("modal-overlay")) closeModal();
});

// manual form extra fields
function renderExtraFields() {
  const tp = $("m-type").value;
  const box = $("m-extra");
  box.innerHTML = "";
  (EXTRA[tp] || []).forEach(([key, label]) => {
    box.insertAdjacentHTML("beforeend",
      `<div class="input-group"><label>${esc(label)}</label><input type="text" data-field="${esc(key)}"/></div>`);
  });
}
$("m-type").addEventListener("change", renderExtraFields);

// import handlers
$("btn-import").addEventListener("click", async () => {
  const text = $("import-area").value;
  if (!text.trim()) return;
  const res = await api().AddLinks(text);
  $("import-area").value = "";
  const box = $("import-errors");
  box.innerHTML = "";
  if (res.added.length) {
    box.insertAdjacentHTML("beforeend", `<div class="ok">✓ ${t("toast.added")}: ${res.added.length}</div>`);
    showToast(`✓ ${t("toast.added")}: ${res.added.length}`);
  }
  (res.errors || []).forEach((e) => box.insertAdjacentHTML("beforeend", `<div class="err">✕ ${esc(e.line)}: ${esc(e.error)}</div>`));
  await loadProfiles();
});

$("btn-import-toml").addEventListener("click", async () => {
  const text = $("toml-area").value;
  if (!text.trim()) return;
  const box = $("toml-status");
  box.innerHTML = "";
  try {
    const p = await api().ImportTOML(text);
    $("toml-area").value = "";
    box.insertAdjacentHTML("beforeend", `<div class="ok">✓ ${esc(p.name)}</div>`);
    showToast("✓ " + p.name);
    await loadProfiles();
  } catch (e) {
    box.insertAdjacentHTML("beforeend", `<div class="err">✕ ${esc((e && e.message) || e)}</div>`);
  }
});

$("btn-add-manual").addEventListener("click", async () => {
  const fields = {};
  document.querySelectorAll("#m-extra input[data-field]").forEach((i) => { if (i.value.trim()) fields[i.dataset.field] = i.value.trim(); });
  try {
    const p = await api().AddManual({
      type: $("m-type").value, name: $("m-name").value.trim(),
      host: $("m-host").value.trim(), port: parseInt($("m-port").value, 10) || 0,
      fields,
    });
    $("m-name").value = ""; $("m-host").value = ""; $("m-port").value = "";
    showToast("✓ " + p.name);
    await loadProfiles();
  } catch (e) { showToast((e && e.message) || e); }
});

// language switch lives in settings; re-render on change of the select
document.addEventListener("change", async (ev) => {
  if (ev.target && ev.target.id === "s-lang") {
    lang = ev.target.value;
    renderContent();
    await fillSettingsForm();
  }
});

// ---------- boot ----------
whenReady(async () => {
  document.querySelectorAll("[data-icon]").forEach((el) => { el.innerHTML = I[el.dataset.icon] || ""; });
  document.querySelectorAll("[data-ph]").forEach((el) => { el.placeholder = t(el.dataset.ph); });
  window.runtime.EventsOn("state", (st) => renderState(st));
  window.runtime.EventsOn("log", () => { if (S.tab === "logs") pullLogs(); });
  window.runtime.EventsOn("appupdate", (info) => {
    S.upd = info;
    showToast(t("set.updAvail") + ": v" + (info && info.latest));
    if (S.tab === "settings") renderContent();
  });
  // «Открыть логи» on the diagnostics page: the 2s pullWDiag poll rebuilds
  // #diag-root (and this button) whenever the feed changes, which strips
  // any listener bound in wireContent — delegate at the document level
  // instead, so the binding survives re-renders.
  document.addEventListener("click", (ev) => {
    if (ev.target && ev.target.closest && ev.target.closest("#btn-diag-logs")) switchTab("logs");
  });
  window.runtime.EventsOn("extip", (d) => {
    S.extIP = (d && d.ip) || "";
    const el = $("stat-ip");
    if (el) el.textContent = S.extIP || ((d && d.error) ? "✕" : "—");
  });
  // The backend auto-pinger emits this; don't wipe an open settings form.
  window.runtime.EventsOn("profiles", async () => {
    S.profiles = (await api().ListProfiles()) || [];
    if (S.tab === "profiles" || S.tab === "vpn") renderContent();
    else renderSidebarFooter();
  });
  await loadSettings();
  await loadProfiles();
  await loadSubs();
  S.version = await api().Version();
  renderState(await api().GetState());
  renderExtraFields();
});
