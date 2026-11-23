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

// ---------- inline svg icons (no CDN: the app must work offline) ----------
const I = {
  shield: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>',
  layers: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>',
  terminal: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>',
  gear: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>',
  power: '<svg class="power-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18.36 6.64a9 9 0 1 1-12.73 0"/><line x1="12" y1="2" x2="12" y2="12"/></svg>',
  checkStroke: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><polyline points="4.5 12.5 9.5 17.5 19.5 7"/></svg>',
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
  share: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/></svg>',
  edit: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4z"/></svg>',
  search: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>',
  zap: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>',
  book: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>',
};

const THEMES = [
  { id: "lilac", name: "Lilac", color: "#a78bfa" },
  { id: "neon-dark", name: "Neon Dark", color: "#00e8a2" },
  { id: "warm-sand", name: "Warm Sand", color: "#e8a040" },
  { id: "arctic", name: "Arctic", color: "#2a64b4" },
  { id: "sunset", name: "Sunset", color: "#f06888" },
  { id: "matrix", name: "Matrix", color: "#00ff41" },
  { id: "mono", name: "Mono Red", color: "#e02020" },
  { id: "custom", name: "Custom", color: "#9aa0a6" },
];

// Fallback palette for the custom-theme editor (mirrors backend fillDefaults).
const DEFAULT_CUSTOM = { accent: "#00e8a2", bg: "#0d0d12", card: "#14141a", fg: "#e0e8f0", muted: "#6b6b78" };

// ---------- i18n ----------
const I18N = {
  ru: {
    "brand": "Wisteria",
    "nav.vpn": "VPN", "nav.profiles": "Профили", "nav.logs": "Логи", "nav.diag": "Диагностика", "nav.settings": "Настройки",
    "vpn.unprotected": "UNPROTECTED", "vpn.protected": "SECURED", "vpn.connecting": "CONNECTING…",
    "vpn.reconnecting": "RECONNECTING…", "vpn.attempt": "попытка {n}",
    "vpn.connect": "Connect", "vpn.disconnect": "Disconnect", "vpn.connected": "Connected",
    "vpn.noProfile": "Профиль не выбран", "vpn.clickSelect": "Нажмите, чтобы выбрать",
    "stat.uptime": "Аптайм", "stat.mode": "Режим", "stat.core": "Ядро", "mode.proxy": "Системный прокси",
    "profiles.title": "Профили", "profiles.empty": "Нет профилей — добавьте ссылку или TOML-конфиг",
    "settings.title": "Настройки", "sec.appearance": "Оформление", "sec.connection": "Соединение",
    "sec.general": "Общие", "sec.routing": "Маршрутизация и DNS", "sec.cores": "Ядра", "sec.system": "Система",
    "set.lang": "Язык", "set.socksPort": "Локальный SOCKS-порт", "set.httpPort": "Локальный HTTP-порт",
    "set.sysproxy": "Системный прокси при подключении", "set.bridge": "HTTP-мост (нужен для системного прокси)",
    "set.autoconnect": "Автоподключение при старте", "set.autostart": "Запускать при входе в систему",
    "set.wisteriaCore": "Путь к wisteria-client", "set.singboxCore": "Путь к sing-box",
    "set.detect": "Найти ядра", "set.save": "Сохранить настройки",
    "set.startMin": "Запускать свёрнутым", "set.startMinNote": "окно свернётся в панель задач",
    "set.confirmExit": "Спрашивать подтверждение при выходе во время соединения",
    "set.dlWintun": "Скачать wintun.dll",
    "set.autoping": "Автопинг серверов, сек", "set.autopingNote": "0 — выключено",
    "set.listenAll": "Разрешить доступ из локальной сети", "set.listenAllNote": "прокси слушает 0.0.0.0 вместо 127.0.0.1",
    "set.bypassLan": "Обходить локальные адреса",
    "sec.wistRouting": "Маршрутизация wisteria",
    "set.bypassRu": "Не проксировать российские сайты",
    "set.bypassRuNote": "цена: сайты из РФ увидят ваш реальный IP, а домены вне встроенного списка (.ru/.su/.рф) резолвятся локально",
    "set.remoteDns": "Удалённый DNS", "set.remoteDnsNote": "например tcp://8.8.8.8",
    "set.logLevel": "Уровень логирования",
    "set.memLimit": "Лимит памяти ядра, МБ", "set.memLimitNote": "0 — без лимита",
    "set.checkIpNow": "Проверить IP сейчас",
    "toast.ipFail": "Не удалось определить IP",
    "toast.ipRetry": "Туннель переподключается — IP появится через несколько секунд",
    "vpn.disconnected": "Disconnected",
    "vpn.preset": "Пресет обфускации",
    "set.wpreset": "Пресет wisteria по умолчанию", "set.wpresetNote": "для профилей без своего пресета; применится при следующем подключении",
    "set.scale": "Масштаб интерфейса", "set.scaleNote": "увеличивает шрифты и элементы на больших экранах",
    "set.customTheme": "Своя тема", "set.ctAccent": "Акцент", "set.ctBg": "Фон", "set.ctCard": "Карточки", "set.ctFg": "Текст", "set.ctMuted": "Второстепенный текст",
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
    "toast.coreDl": "Скачивание…",
    "set.ghToken": "GitHub-токен", "set.ghTokenNote": "нужен только для ядра wisteria (приватный репозиторий); хранится в settings.json с правами 0600",
    "toast.updNone": "актуальная версия",
    "toast.updReady": "Обновление загружено — можно перезапускать",
    "set.restartAdmin": "Перезапустить от администратора",
    "set.notAdmin": "Без прав администратора TUN недоступен.",
    "tun.needSingbox": "Для TUN нужно ядро sing-box",
    "tun.needWintun": "Для TUN нужен wintun.dll",
    "tun.needAdmin": "Для TUN нужны права администратора",
    "tun.needCap": "Для TUN нужны права на сетевой интерфейс",
    "tun.dlSingbox": "Скачать sing-box",
    "tun.dlWintun": "Скачать wintun.dll",
    "tun.restartAdmin": "Перезапустить от администратора",
    "tun.copy": "Скопировать",
    "tun.copied": "Команда скопирована",
    "tun.notReady": "TUN не готов: {why}",
    "vpn.cancel": "Отмена",
    "err.openLogs": "Открыть логи",
    "set.mpCore": "Ядро для ss/vmess/vless/trojan",
    "set.xrayCore": "Путь к xray",
    "logs.title": "Логи", "logs.follow": "Автопрокрутка", "logs.clear": "Очистить",
    "logs.copy": "Скопировать", "logs.copied": "Логи скопированы ({n} строк)",
    "logs.copyFail": "Не удалось скопировать логи", "logs.copyEmpty": "Логов пока нет",
    "logs.healthIdle": "туннель не активен", "logs.healthTip": "Последний health-срез ядра: задержка, потери, число проверок",
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
    "lbl.transport": "Транспорт", "lbl.tlsSni": "TLS SNI", "lbl.tlsSniNote": "по умолчанию — хост сервера",
    "lbl.tlsInsecure": "Не проверять сертификат",
    "tls.sniAuto": "SNI: хост сервера",
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
    "toast.clipSubs": "подписок: {n}", "toast.clipProfiles": "профилей: {n}", "toast.clipNone": "Ничего нового в буфере",
    "toast.stuck": "Подключение зависло — попробуйте ещё раз",
    "profiles.search": "Поиск по имени или адресу…",
    "profiles.sortDefault": "Как добавлены", "profiles.sortName": "По имени", "profiles.sortPing": "По пингу",
    "profiles.pingAll": "Пинговать все", "profiles.fastest": "Самый быстрый",
    "profiles.share": "Поделиться", "profiles.edit": "Изменить",
    "profiles.udpPing": "udp · без TCP-пинга",
    "profiles.udpPingNote": "Hysteria2/TUIC работают по UDP — TCP-пинг к ним неприменим и ничего не говорит о подключении",
    "profiles.noRaw": "У профиля нет исходной ссылки",
    "profiles.fastestNone": "Нет профилей с известным пингом — сначала пропингуйте",
    "profiles.fastestDone": "Выбран «{name}» · {ms} ms",
    "edit.title": "Изменить профиль", "edit.save": "Сохранить",
    "qr.title": "Поделиться профилем", "qr.hint": "Отсканируйте камерой телефона", "qr.copy": "Копировать ссылку",
    "toast.copied": "Скопировано",
    "auto.name": "Автопрофиль",
    "auto.desc": "Автоматически выбирает лучший сервер и переключается при сбое",
    "auto.activeSub": "Активен: {name}",
    "upd.modalTitle": "Доступно обновление",
    "upd.modalDesc": "Вышла новая версия Wisteria. Рекомендуем обновиться для получения исправлений и новых функций.",
    "upd.later": "Позже",
    "set.hwid": "ID устройства (HWID)",
    "set.hwidNote": "аппаратный идентификатор для контроля лимита устройств в подписках",
    "set.shortcuts": "Ярлыки в системе (Linux)",
    "set.shortcutsNote": "добавить в меню приложений и на рабочий стол",
    "set.shortcutsBtn": "Создать ярлыки",
    "toast.shortcutsOk": "Ярлыки созданы",
    "guide.btn": "Справочник и FAQ",
    "guide.title": "Справочник и FAQ",
    "guide.tabCores": "Ядра",
    "guide.tabUpdates": "Обновления",
    "guide.tabFeatures": "Возможности",
    "guide.tabFaq": "FAQ",
  },
  en: {
    "brand": "Wisteria",
    "nav.vpn": "VPN", "nav.profiles": "Profiles", "nav.logs": "Logs", "nav.diag": "Diagnostics", "nav.settings": "Settings",
    "vpn.unprotected": "UNPROTECTED", "vpn.protected": "SECURED", "vpn.connecting": "CONNECTING…",
    "vpn.reconnecting": "RECONNECTING…", "vpn.attempt": "attempt {n}",
    "vpn.connect": "Connect", "vpn.disconnect": "Disconnect", "vpn.connected": "Connected",
    "vpn.noProfile": "No profile selected", "vpn.clickSelect": "Click to choose",
    "stat.uptime": "Uptime", "stat.mode": "Mode", "stat.core": "Core", "mode.proxy": "System proxy",
    "profiles.title": "Profiles", "profiles.empty": "No profiles yet — add a link or a TOML config",
    "settings.title": "Settings", "sec.appearance": "Appearance", "sec.connection": "Connection",
    "sec.general": "General", "sec.routing": "Routing & DNS", "sec.cores": "Cores", "sec.system": "System",
    "set.lang": "Language", "set.socksPort": "Local SOCKS port", "set.httpPort": "Local HTTP port",
    "set.sysproxy": "Set system proxy on connect", "set.bridge": "HTTP bridge (required for the system proxy)",
    "set.autoconnect": "Auto-connect on start", "set.autostart": "Start on login",
    "set.wisteriaCore": "Path to wisteria-client", "set.singboxCore": "Path to sing-box",
    "set.detect": "Detect cores", "set.save": "Save settings",
    "set.startMin": "Start minimized", "set.startMinNote": "the window opens minimized",
    "set.confirmExit": "Ask for confirmation on exit while connected",
    "set.dlWintun": "Download wintun.dll",
    "set.autoping": "Auto-ping servers, sec", "set.autopingNote": "0 — disabled",
    "set.listenAll": "Allow LAN access", "set.listenAllNote": "proxy listens on 0.0.0.0 instead of 127.0.0.1",
    "set.bypassLan": "Bypass local addresses",
    "sec.wistRouting": "Wisteria routing",
    "set.bypassRu": "Do not proxy Russian sites",
    "set.bypassRuNote": "the price: Russian sites see your real IP, and domains outside the built-in list (.ru/.su/.рф) are resolved locally",
    "set.remoteDns": "Remote DNS", "set.remoteDnsNote": "e.g. tcp://8.8.8.8",
    "set.logLevel": "Log level",
    "set.memLimit": "Core memory limit, MB", "set.memLimitNote": "0 — unlimited",
    "set.checkIpNow": "Check IP now",
    "toast.ipFail": "Could not determine the IP",
    "toast.ipRetry": "Tunnel is reconnecting — the IP will appear in a few seconds",
    "vpn.disconnected": "Disconnected",
    "vpn.preset": "Obfuscation preset",
    "set.wpreset": "Default wisteria preset", "set.wpresetNote": "for profiles without their own preset; applies on next connect",
    "set.scale": "Interface scale", "set.scaleNote": "enlarges fonts and controls on big screens",
    "set.customTheme": "Custom theme", "set.ctAccent": "Accent", "set.ctBg": "Background", "set.ctCard": "Cards", "set.ctFg": "Text", "set.ctMuted": "Muted text",
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
    "toast.coreDl": "Downloading…",
    "set.ghToken": "GitHub token", "set.ghTokenNote": "only needed for the wisteria core (private repo); stored in settings.json with 0600 permissions",
    "toast.updNone": "up to date",
    "toast.updReady": "Update staged — restart to apply",
    "set.restartAdmin": "Restart as administrator",
    "set.notAdmin": "TUN needs administrator rights.",
    "tun.needSingbox": "TUN requires sing-box",
    "tun.needWintun": "TUN requires wintun.dll",
    "tun.needAdmin": "TUN requires administrator rights",
    "tun.needCap": "TUN needs network-interface rights",
    "tun.dlSingbox": "Download sing-box",
    "tun.dlWintun": "Download wintun.dll",
    "tun.restartAdmin": "Restart as administrator",
    "tun.copy": "Copy",
    "tun.copied": "Command copied",
    "tun.notReady": "TUN is not ready: {why}",
    "vpn.cancel": "Cancel",
    "err.openLogs": "Open logs",
    "set.mpCore": "Engine for ss/vmess/vless/trojan",
    "set.xrayCore": "Path to xray",
    "logs.title": "Logs", "logs.follow": "Auto-scroll", "logs.clear": "Clear",
    "logs.copy": "Copy", "logs.copied": "Logs copied ({n} lines)",
    "logs.copyFail": "Could not copy the logs", "logs.copyEmpty": "No logs yet",
    "logs.healthIdle": "tunnel inactive", "logs.healthTip": "Latest core health slice: latency, loss, probe count",
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
    "lbl.transport": "Transport", "lbl.tlsSni": "TLS SNI", "lbl.tlsSniNote": "defaults to the server host",
    "lbl.tlsInsecure": "Skip certificate verification",
    "tls.sniAuto": "SNI: server host",
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
    "toast.clipSubs": "subscriptions: {n}", "toast.clipProfiles": "profiles: {n}", "toast.clipNone": "Nothing new in the clipboard",
    "toast.stuck": "Connection is stuck — please try again",
    "profiles.search": "Search by name or address…",
    "profiles.sortDefault": "As added", "profiles.sortName": "By name", "profiles.sortPing": "By ping",
    "profiles.pingAll": "Ping all", "profiles.fastest": "Fastest",
    "profiles.share": "Share", "profiles.edit": "Edit",
    "profiles.udpPing": "udp · no TCP ping",
    "profiles.udpPingNote": "Hysteria2/TUIC run over UDP — a TCP ping does not apply and says nothing about the tunnel",
    "profiles.noRaw": "This profile has no original link",
    "profiles.fastestNone": "No profile has a known ping — ping them first",
    "profiles.fastestDone": "Selected «{name}» · {ms} ms",
    "edit.title": "Edit profile", "edit.save": "Save",
    "qr.title": "Share profile", "qr.hint": "Scan with your phone camera", "qr.copy": "Copy link",
    "toast.copied": "Copied",
    "auto.name": "Auto-Profile",
    "auto.desc": "Automatically picks the best server and fails over on drop",
    "auto.activeSub": "Active: {name}",
    "upd.modalTitle": "Update available",
    "upd.modalDesc": "A new version of Wisteria is available. Update now to get the latest fixes and improvements.",
    "upd.later": "Later",
    "set.hwid": "Device ID (HWID)",
    "set.hwidNote": "hardware identifier used for per-link device limits in subscriptions",
    "set.shortcuts": "System shortcuts (Linux)",
    "set.shortcutsNote": "install into application menu and desktop",
    "set.shortcutsBtn": "Create shortcuts",
    "toast.shortcutsOk": "Shortcuts created",
    "guide.btn": "Guide & FAQ",
    "guide.title": "Guide & FAQ",
    "guide.tabCores": "Cores",
    "guide.tabUpdates": "Updates",
    "guide.tabFeatures": "Features",
    "guide.tabFaq": "FAQ",
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
  tab: "vpn", setTab: "look", theme: "lilac", customTheme: null, uiScale: 100, profiles: [], selected: "", lastLogSeq: 0,
  pfFilter: "", pfSort: "default", pfPingBusy: false, pendingShare: "",
  state: { state: "disconnected" }, uptimeTimer: null,
  subs: [], subBusy: {}, busy: false, // busy = a Connect is in flight
  version: "", upd: null, updBusy: false, updReady: false, coreUpd: {},
  tunStatus: null, // last TunPreflight snapshot (VPN tab card)
  wdiag: null, // the Diagnostics page data (wisteria profiles only)
  devInfo: null, // hardware device id (HWID)
  coreInfo: null, // detected OS and cores
  guideTab: "cores",
  updModalShown: false,
};
const $ = (id) => document.getElementById(id);
const esc = (s) => String(s == null ? "" : s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const typeIcon = (tp) => ({ wisteria: I.lock }[tp] || I.globe);
// Hysteria2/TUIC are QUIC/UDP: a TCP connect probe always fails there, so
// the list must not render a working server as "ping n/a" (red/dead).
const udpProto = (p) => !!p && (p.type === "hysteria2" || p.type === "tuic");

// TLS badge for wisteria profiles on a real-TLS transport (wave 4). The
// tooltip names the SNI override when one is set, otherwise the default
// (the endpoint host).
function tlsBadge(p) {
  if (!p || p.type !== "wisteria" || p.transport !== "tls") return "";
  const tip = p.tls_sni ? "SNI: " + p.tls_sni : t("tls.sniAuto");
  return ` <span class="tls-badge" title="${esc(tip)}">TLS</span>`;
}

function protoBadge(p) {
  if (!p) return "";
  if (p.id === "__auto__") return ' <span class="proto-badge pb-auto">AUTO</span>';
  const raw = String(p.type || "").toLowerCase();
  if (!raw) return "";
  const label = raw === "hysteria2" ? "HY2" : raw.toUpperCase();
  return ` <span class="proto-badge pb-${esc(raw)}">${esc(label)}</span>`;
}

function showToast(msg) {
  const el = $("toast");
  el.textContent = msg;
  el.classList.add("show");
  clearTimeout(el._timer);
  el._timer = setTimeout(() => el.classList.remove("show"), 2500);
}

// Haptic feedback on the phone (WebView honours navigator.vibrate with
// the VIBRATE permission); a no-op on desktop. Short and rare: connect
// tap, successful connect, failure — never on a timer.
const buzz = (p) => { try { if (navigator.vibrate) navigator.vibrate(p); } catch (e) { /* unsupported */ } };

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
  if (tab === "logs") { pullLogs(); pullLogHealth(); }
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
  if (S.tab === "vpn" && S.cfg && S.cfg.tun_enabled) refreshTunPreflight();
}

function renderSidebarFooter() {
  const st = S.state;
  // Before the first connect the backend status carries no profile_id —
  // fall back to the user's selected (active) profile so the choice made
  // on the profiles tab is visible immediately, not only after Connect.
  const isAuto = S.selected === "__auto__" || !!st.auto_profile;
  const p = S.profiles.find((x) => x.id === (st.profile_id || S.selected));
  const dot = st.state === "connected" ? "on" : (st.state === "connecting" || st.reconnecting) ? "mid" : "";
  const statusText = st.state === "connected" ? t("vpn.connected") : st.reconnecting ? t("vpn.reconnecting") + (st.attempt ? " " + t("vpn.attempt").replace("{n}", st.attempt) : "") : st.state === "connecting" ? t("vpn.connecting") : t("vpn.unprotected");
  const title = isAuto ? (p ? t("auto.name") + " · " + p.name : t("auto.name")) : (p ? p.name : (st.profile_name || "—"));
  const sub = p ? p.host + ":" + p.port : (isAuto ? t("auto.desc") : (st.socks_addr || ""));
  $("sidebar-footer").innerHTML = `
    <div class="sf-status"><div class="sf-dot ${dot}"></div> ${esc(statusText)}</div>
    <div class="sf-server">${esc(title)}</div>
    <div class="sf-ip">${esc(sub)}</div>`;
}

function renderVpn() {
  const st = S.state;
  const conn = st.state === "connected";
  const connecting = st.state === "connecting";
  const reconn = !!st.reconnecting;
  // Same as the sidebar footer: show the selected profile before the
  // backend status ever mentions one (i.e. before the first Connect).
  const isAuto = S.selected === "__auto__" || !!st.auto_profile;
  const p = S.profiles.find((x) => x.id === (st.profile_id || S.selected)) || null;
  const label = conn ? t("vpn.protected") : reconn ? t("vpn.reconnecting") + (st.attempt ? " " + t("vpn.attempt").replace("{n}", st.attempt) : "") : connecting ? t("vpn.connecting") : t("vpn.unprotected");
  // Connecting and reconnecting are cancellable, so the button says
  // "Отмена" instead of repeating the status the label above already shows.
  const btnText = conn ? t("vpn.connected") : (connecting || reconn) ? t("vpn.cancel") : t("vpn.disconnected");
  const lat = p && p.latency_ms ? (p.latency_ms < 0 ? "n/a" : p.latency_ms + " ms") : "";
  const tun = !!(S.cfg && S.cfg.tun_enabled);
  const cardIcon = isAuto ? I.zap : typeIcon(p ? p.type : "");
  const cardTitle = isAuto
    ? esc(t("auto.name")) + protoBadge({ id: "__auto__" }) + (p ? protoBadge(p) : "")
    : esc(p ? p.name : t("vpn.noProfile")) + protoBadge(p) + tlsBadge(p);
  const cardSub = isAuto
    ? (p ? t("auto.activeSub").replace("{name}", p.name) + " · " + p.host + ":" + p.port + (lat ? " · " + lat : "") : t("auto.desc"))
    : (p ? p.host + ":" + p.port + (lat ? " · " + lat : "") : t("vpn.clickSelect"));
  return `
    <div class="vpn-screen">
      <div class="vpn-status ${conn ? "on" : connecting || reconn ? "mid" : ""}"><div class="label">${esc(label)}</div></div>
      <div id="btn-wrap" style="position:relative; display:flex; justify-content:center;">
        <button class="connect-btn ${conn ? "connected" : ""} ${connecting || reconn ? "connecting" : ""}" id="btn-toggle">
          <div class="ring-anim"></div>
          <div class="ring-anim d2"></div>
          <svg class="spin-arc" viewBox="0 0 100 100" aria-hidden="true"><circle cx="50" cy="50" r="46"/></svg>
          <span class="btn-icons">${I.power}<span class="icon-check">${I.checkStroke}</span></span>
          <span class="connect-text">${esc(btnText)}</span>
        </button>
      </div>
      <div class="profile-card" id="card-profile">
        <div class="icon-box">${cardIcon}</div>
        <div class="srv-info">
          <div class="srv-name">${cardTitle}</div>
          <div class="srv-ping">${esc(cardSub)}</div>
        </div>
        ${I.chev}
      </div>
      <div class="stats-row">
        <div class="stat-card"><div class="stat-label">${I.clock} ${esc(t("stat.uptime"))}</div><div class="stat-value" id="stat-uptime">${uptimeHTML(st.started_at) || "—"}</div></div>
        <div class="stat-card mode-card">
          <div class="stat-label">${I.activity} ${esc(t("stat.mode"))}</div>
          <fieldset class="seg">
            <legend class="vh">${esc(t("stat.mode"))}</legend>
            <div class="seg-track" id="seg-track" data-active="${tun ? "tun" : "proxy"}">
              <span class="seg-thumb" aria-hidden="true"></span>
              <label class="seg-item"><input type="radio" name="mode" value="proxy" ${tun ? "" : "checked"}/><span class="seg-opt">${I.globe}${esc(t("mode.proxy"))}</span></label>
              <label class="seg-item"><input type="radio" name="mode" value="tun" ${tun ? "checked" : ""}/><span class="seg-opt">${I.layers}TUN</span></label>
            </div>
          </fieldset>
        </div>
      </div>
      ${tun ? `<div id="tun-preflight">${tunPreflightHTML(S.tunStatus)}</div>` : ""}
      ${p && p.type === "wisteria" ? `
      <div class="vpn-tun-row">
        <div class="vpn-tun-label">${esc(t("vpn.preset"))}</div>
        <select id="vpn-preset" class="preset-select">
          ${["fast", "balance", "stealth"].map((k) => `<option value="${k}" ${((p.fields && p.fields.preset) || (S.cfg && S.cfg.wisteria_preset) || "stealth") === k ? "selected" : ""}>${k}</option>`).join("")}
        </select>
      </div>` : ""}
      ${st.error ? `<div class="error-box">${esc(st.error)}<button class="ghost-btn err-logs-btn" id="btn-err-logs">${esc(t("err.openLogs"))}</button></div>` : ""}
    </div>`;
}


// ---------- TUN preflight ----------
// The card lists only what is actually missing, each row with the one
// click that fixes it. The backend mirrors the startTun gate, so this can
// never promise something Connect would still reject.
function tunPreflightHTML(pf) {
  if (!pf || pf.ready) return "";
  const rows = [];
  if (!pf.singbox_found) {
    rows.push(`<div class="tun-pf-row"><span>${esc(t("tun.needSingbox"))}</span>` +
      `<button class="ghost-btn" data-tun-act="singbox">${esc(t("tun.dlSingbox"))}</button></div>`);
  } else if (pf.os === "windows" && !pf.wintun_found) {
    // Downloading sing-box also installs wintun.dll, so this row only
    // appears when the core exists and the DLL is still missing.
    rows.push(`<div class="tun-pf-row"><span>${esc(t("tun.needWintun"))}</span>` +
      `<button class="ghost-btn" data-tun-act="wintun">${esc(t("tun.dlWintun"))}</button></div>`);
  }
  if (pf.os === "windows" && !pf.elevated) {
    rows.push(`<div class="tun-pf-row"><span>${esc(t("tun.needAdmin"))}</span>` +
      `<button class="ghost-btn" data-tun-act="admin">${esc(t("tun.restartAdmin"))}</button></div>`);
  }
  if (pf.os === "linux" && pf.singbox_found && !pf.has_cap) {
    const cmd = "sudo setcap cap_net_admin=+ep " + (pf.singbox_path || "<путь к sing-box>");
    rows.push(`<div class="tun-pf-row"><span>${esc(t("tun.needCap"))}</span>` +
      `<button class="ghost-btn" data-tun-act="cap">${esc(t("tun.copy"))}</button></div>` +
      `<code class="tun-pf-cmd">${esc(cmd)}</code>`);
  }
  if (!rows.length) return "";
  return `<div class="tun-preflight"><div class="tun-pf-title">${esc(t("set.tun"))}</div>${rows.join("")}</div>`;
}

function tunMissingShort(pf) {
  if (!pf || !pf.singbox_found) return t("tun.needSingbox");
  if (pf.os === "windows" && !pf.wintun_found) return t("tun.needWintun");
  if (pf.os === "windows" && !pf.elevated) return t("tun.needAdmin");
  if (pf.os === "linux" && !pf.has_cap) return t("tun.needCap");
  return "";
}

// Fetches the snapshot and patches the card in place: a full render here
// would recreate the connect button mid-animation.
async function refreshTunPreflight() {
  let pf = null;
  try { pf = await api().TunPreflight(); } catch (e) { pf = null; }
  S.tunStatus = pf;
  const box = $("tun-preflight");
  if (box) {
    const sig = JSON.stringify(pf);
    if (box._sig !== sig) {
      box.innerHTML = tunPreflightHTML(pf);
      box._sig = sig;
    }
  }
  return pf;
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

function matchProfile(p) {
  const f = S.pfFilter.trim().toLowerCase();
  if (!f) return true;
  return String(p.name || "").toLowerCase().includes(f) ||
    (String(p.host || "") + ":" + (p.port || "")).toLowerCase().includes(f);
}

function sortProfiles(list) {
  const arr = list.slice();
  if (S.pfSort === "name") {
    arr.sort((a, b) => String(a.name || "").localeCompare(String(b.name || ""), lang));
  } else if (S.pfSort === "ping") {
    const rank = (p) => { if (udpProto(p)) return 2e9; const l = Number(p.latency_ms) || 0; return l > 0 ? l : l < 0 ? 1e9 : 2e9; };
    arr.sort((a, b) => rank(a) - rank(b));
  }
  return arr;
}

function profileCard(p) {
  const active = p.id === S.selected;
  const udp = udpProto(p);
  const lat = udp
    ? `<div class="sli-lat" title="${esc(t("profiles.udpPingNote"))}">${esc(t("profiles.udpPing"))}</div>`
    : p.latency_ms
      ? `<div class="sli-lat ${p.latency_ms > 0 ? "good" : ""}">${p.latency_ms < 0 ? "ping n/a" : "ping " + p.latency_ms + " ms"}</div>`
      : "";
  return `
    <div class="server-list-item ${active ? "active-server" : ""}" data-id="${p.id}">
      <div class="sli-icon">${typeIcon(p.type)}</div>
      <div class="sli-data">
        <div class="sli-name">${esc(p.name)}${protoBadge(p)}${tlsBadge(p)}</div>
        <div class="sli-addr">${esc(p.host)}:${p.port}</div>
        ${lat}
      </div>
      ${p.raw ? `<button class="sli-act shareb" title="${esc(t("profiles.share"))}" data-id="${p.id}">${I.share}</button>` : ""}
      ${p.type === "wisteria" ? `<button class="sli-act editb" title="${esc(t("profiles.edit"))}" data-id="${p.id}">${I.edit}</button>` : ""}
      <button class="sli-act pingb" title="ping" data-id="${p.id}" style="${udp ? "display:none" : ""}">${I.ping}</button>
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

function renderAutoProfileCard() {
  if (!S.profiles.length) return "";
  const active = S.selected === "__auto__" || !!S.state.auto_profile;
  const live = S.state.auto_profile ? S.profiles.find((x) => x.id === S.state.profile_id) : null;
  const desc = live ? t("auto.activeSub").replace("{name}", live.name) : t("auto.desc");
  return `
    <div class="server-list-item auto-profile-card ${active ? "active-server" : ""}" data-id="__auto__">
      <div class="sli-icon">${I.zap}</div>
      <div class="sli-data">
        <div class="sli-name">${esc(t("auto.name"))}${protoBadge({ id: "__auto__" })}</div>
        <div class="sli-addr">${esc(desc)}</div>
      </div>
      <span class="sli-check" style="${active ? "" : "display:none"}">${I.check}</span>
    </div>`;
}

function renderProfilesList() {
  const manual = S.profiles.filter((p) => !p.sub_id || !S.subs.some((x) => x.id === p.sub_id));
  const visible = (list) => sortProfiles(list.filter(matchProfile));
  const groups = S.subs.map((s) => {
    const meta = s.error
      ? `<div class="sub-meta sub-err">${esc(s.error)}</div>`
      : `<div class="sub-meta">${esc(t("subs.updated"))}: ${esc(fmtSubTime(s.updated_at))}</div>`;
    const actions = `
      <button class="sli-act sub-refresh ${S.subBusy[s.id] ? "spin" : ""}" title="${esc(t("subs.refresh"))}" data-id="${s.id}">${I.refresh}</button>
      <button class="sli-act del" title="delete" data-id="${s.id}">${I.trash}</button>`;
    return renderGroup("sub:" + s.id, s.name, meta,
      visible(S.profiles.filter((p) => p.sub_id === s.id)), "subs.emptyGroup", actions);
  }).join("");
  return renderAutoProfileCard() + groups + (S.subs.length
    ? renderGroup("manual", t("profiles.manual"), "", visible(manual), "profiles.empty", "")
    : profileGrid(visible(manual), "profiles.empty"));
}

function renderProfiles() {
  return `
    <div class="servers-header">
      <h2>${esc(t("profiles.title"))}</h2>
      <div class="header-actions">
        <button class="ghost-btn" id="btn-clip" title="${esc(t("btn.clipTip"))}">${I.clipboard}<span>${esc(t("btn.clip"))}</span></button>
        <button class="add-btn" id="btn-add" title="${esc(t("add.title"))}">${I.plus}</button>
      </div>
    </div>
    <div class="pf-toolbar">
      <div class="pf-search">${I.search}<input id="pf-search" type="search" value="${esc(S.pfFilter)}" placeholder="${esc(t("profiles.search"))}"/></div>
      <button class="ghost-btn" id="btn-ping-all" ${S.pfPingBusy ? "disabled" : ""}>${I.ping}<span>${esc(t("profiles.pingAll"))}</span></button>
      <button class="ghost-btn" id="btn-fastest">${I.zap}<span>${esc(t("profiles.fastest"))}</span></button>
      <select id="pf-sort" class="pf-sort">
        <option value="default" ${S.pfSort === "default" ? "selected" : ""}>${esc(t("profiles.sortDefault"))}</option>
        <option value="name" ${S.pfSort === "name" ? "selected" : ""}>${esc(t("profiles.sortName"))}</option>
        <option value="ping" ${S.pfSort === "ping" ? "selected" : ""}>${esc(t("profiles.sortPing"))}</option>
      </select>
    </div>
    <div id="pf-list">${renderProfilesList()}</div>`;
}

function renderLogs() {
  return `
    <div class="logs-header">
      <h2>${esc(t("logs.title"))}</h2>
      <div class="logs-tools">
        <button class="ghost-btn" id="btn-copy-logs">${esc(t("logs.copy"))}</button>
        <button class="ghost-btn" id="btn-clear-logs">${esc(t("logs.clear"))}</button>
      </div>
    </div>
    <div class="logs-bar">
      <label class="log-follow"><span class="switch"><input type="checkbox" id="log-follow" checked/><span class="slider"></span></span><span>${esc(t("logs.follow"))}</span></label>
      <div class="log-health" id="log-health" title="${esc(t("logs.healthTip"))}"><span class="lh-dot"></span><span id="log-health-text">—</span></div>
    </div>
    <pre class="log-view" id="log-view"></pre>`;
}

// The live health line: the core's periodic "health: rtt=… loss=… pings=…"
// reports are consumed by the backend (they used to flood this screen);
// the freshest slice is painted here instead, polled while Logs is open.
async function pullLogHealth() {
  const el = $("log-health-text");
  if (!el) return;
  try {
    const h = await api().GetCoreHealth();
    const wrap = $("log-health");
    if (h && h.active) {
      if (wrap) wrap.classList.add("live");
      el.textContent = "rtt " + h.rtt_ms + " ms · loss " + h.loss_pct + "% · pings " + h.pings +
        (h.at ? " · " + new Date(h.at).toLocaleTimeString() : "");
    } else {
      if (wrap) wrap.classList.remove("live");
      el.textContent = t("logs.healthIdle");
    }
  } catch (e) { /* bridge not ready / method absent on old builds */ }
}
setInterval(() => { if (!document.hidden && S.tab === "logs") pullLogHealth(); }, 2000);

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
  const ev = d.events || [];
  if (!d.active && !ev.length) {
    return `
      <div class="diag-banner warn"><div class="diag-banner-ic">${I.lock}</div><div><b>${esc(t("diag.inactive"))}</b><p>${esc(t("diag.inactiveHint"))}</p></div></div>
      <button class="ghost-btn" id="btn-diag-logs">${esc(t("diag.openLogs"))}</button>`;
  }
  const [cls, key] = diagBanner(d);
  const srv = d.servers || [];
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
setInterval(() => { if (!document.hidden && S.tab === "diag") pullWDiag(); }, 2000);

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
        <label>${esc(t("set.autoping"))}<span class="set-note">${esc(t("set.autopingNote"))}</span></label>
        <input id="s-autoping" type="number" min="0" max="3600"/>
        <label>${esc(t("set.autoconnect"))}</label>
        <select id="s-autoconnect"><option value="">—</option></select>
        <label>${esc(t("set.hwid"))}<span class="set-note">${esc(t("set.hwidNote"))}</span></label>
        <div class="hwid-row">
          <code class="hwid-code" id="s-hwid">${esc((S.devInfo && S.devInfo.id) || "…")}</code>
          <button class="ghost-btn" id="btn-copy-hwid" type="button" title="${esc(t("tun.copy"))}">${esc(t("tun.copy"))}</button>
        </div>
        ${S.coreInfo && S.coreInfo.os === "linux" ? `
        <label>${esc(t("set.shortcuts"))}<span class="set-note">${esc(t("set.shortcutsNote"))}</span></label>
        <div><button class="ghost-btn" id="btn-shortcuts" type="button">${esc(t("set.shortcutsBtn"))}</button></div>` : ""}
      </div>
      <div style="margin-top:10px; display:flex; gap:10px; flex-wrap:wrap;">
        <button class="ghost-btn" id="btn-checkip">${esc(t("set.checkIpNow"))}</button>
        <button class="ghost-btn" id="btn-open-guide-2">${I.book} <span style="margin-left:6px;">${esc(t("guide.btn"))}</span></button>
      </div>
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
    </div>
    <div class="settings-section">
      <h3>${esc(t("sec.wistRouting"))} <span class="sec-note">wisteria</span></h3>
      <div class="set-grid">
        <label>${esc(t("set.bypassRu"))}<span class="set-note">${esc(t("set.bypassRuNote"))}</span></label>
        <label class="switch"><input id="s-bypassru" type="checkbox"/><span class="slider"></span></label>
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
        <label>${esc(t("set.ghToken"))}<span class="set-note">${esc(t("set.ghTokenNote"))}</span></label>
        <input id="s-ghtoken" type="password" autocomplete="off" placeholder="ghp_… / github_pat_…"/>
        <label>${esc(t("set.memLimit"))}<span class="set-note">${esc(t("set.memLimitNote"))}</span></label>
        <input id="s-memlimit" type="number" min="0" max="16384"/>
      </div>
      <div style="display:flex; gap:10px; margin-top:14px; align-items:center;">
        <button class="ghost-btn" id="btn-detect">${esc(t("set.detect"))}</button>
        <button class="ghost-btn" id="btn-upd-singbox" ${S.coreUpd["sing-box"] ? "disabled" : ""}>${esc(t("set.updCore"))} sing-box</button>
        <button class="ghost-btn hidden" id="btn-upd-wintun" ${S.coreUpd["wintun"] ? "disabled" : ""}>${esc(t("set.dlWintun"))}</button>
        <button class="ghost-btn" id="btn-upd-xray" ${S.coreUpd["xray"] ? "disabled" : ""}>${esc(t("set.updCore"))} xray</button>
        <button class="ghost-btn" id="btn-upd-wisteria" ${S.coreUpd["wisteria"] ? "disabled" : ""}>${esc(t("set.updCore"))} wisteria</button>
      </div>
      <div class="core-note" id="core-note"></div>
    </div>`;
  } else {
    body = `
    <div class="settings-section">
      <h3>${esc(t("sec.appearance"))}</h3>
      <div class="theme-grid">${THEMES.map((th) => `
        <div class="theme-option ${S.theme === th.id ? "selected" : ""}" data-theme="${th.id}">
          <div class="theme-dot ${th.id === "custom" ? "custom-dot" : ""}" style="background:${th.color}"></div>
          <div class="theme-name">${esc(th.name)}</div>
        </div>`).join("")}
      </div>
      ${S.theme === "custom" ? `
      <div class="set-grid ct-grid" style="margin-top:12px;">
        <label>${esc(t("set.ctAccent"))}</label><input type="color" id="ct-accent" value="${(S.customTheme || DEFAULT_CUSTOM).accent}"/>
        <label>${esc(t("set.ctBg"))}</label><input type="color" id="ct-bg" value="${(S.customTheme || DEFAULT_CUSTOM).bg}"/>
        <label>${esc(t("set.ctCard"))}</label><input type="color" id="ct-card" value="${(S.customTheme || DEFAULT_CUSTOM).card}"/>
        <label>${esc(t("set.ctFg"))}</label><input type="color" id="ct-fg" value="${(S.customTheme || DEFAULT_CUSTOM).fg}"/>
        <label>${esc(t("set.ctMuted"))}</label><input type="color" id="ct-muted" value="${(S.customTheme || DEFAULT_CUSTOM).muted}"/>
      </div>` : ""}
      <div class="set-grid" style="margin-top:12px;">
        <label>${esc(t("set.scale"))}<span class="set-note">${esc(t("set.scaleNote"))}</span></label>
        <div class="scale-row"><input type="range" id="s-scale" min="70" max="160" step="5" value="${S.uiScale || 100}"/><span id="s-scale-val">${S.uiScale || 100}%</span></div>
        <label>${esc(t("set.wpreset"))}<span class="set-note">${esc(t("set.wpresetNote"))}</span></label>
        <select id="s-wpreset"><option value="fast">fast</option><option value="balance">balance</option><option value="stealth">stealth</option></select>
        <label>${esc(t("set.lang"))}</label>
        <select id="s-lang"><option value="ru">Русский</option><option value="en">English</option></select>
      </div>
    </div>`;
  }
  return `<div class="settings-head-row"><div class="settings-title">${esc(t("settings.title"))}</div><button class="ghost-btn" id="btn-open-guide">${I.book} <span style="margin-left:6px;">${esc(t("guide.btn"))}</span></button></div>${bar}${body}
    <div style="margin:6px 0 30px;">
      <button class="submit-btn save-btn" id="btn-save-settings">${esc(t("set.save"))}</button>
    </div>`;
}

// Optimistically show "connecting to X" the moment a switch starts, so the
// wait is visible; backend state events overwrite this as they arrive.
function markConnecting(id) {
  if (id === "__auto__") {
    renderState({ ...S.state, state: "connecting", auto_profile: true, profile_name: t("auto.name"), error: "" });
    return;
  }
  const p = S.profiles.find((x) => x.id === id);
  if (!p) return;
  renderState({ ...S.state, state: "connecting", auto_profile: false, profile_id: p.id, profile_name: p.name, error: "" });
}

// Reject after ms so a wedged backend call can never lock the busy guard
// forever (the promise itself may still settle later; state events heal the UI).
function withTimeout(p, ms, msg) {
  return Promise.race([
    p,
    new Promise((_, rej) => setTimeout(() => rej(new Error(msg)), ms)),
  ]);
}

// One-button core download/update. The button itself carries the progress
// ("Скачивание…") — a full renderContent() here would wipe unsaved edits
// in the settings form, so only the button and the detect note change.
async function updateCore(name) {
  if (S.coreUpd[name]) return;
  S.coreUpd[name] = true;
  const btnId = { "sing-box": "btn-upd-singbox", xray: "btn-upd-xray", wisteria: "btn-upd-wisteria" }[name];
  const btn = $(btnId);
  const oldLabel = btn ? btn.textContent : "";
  if (btn) { btn.disabled = true; btn.textContent = t("toast.coreDl"); }
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
  if (btn) { btn.disabled = false; btn.textContent = oldLabel; }
  // A fresh download must show up at once: re-detect, refresh the note and
  // flip "Скачать" → "Обновить" on the buttons that now have a binary.
  try {
    const info = await api().DetectCores();
    const note = $("core-note");
    if (note && info) note.innerHTML = coreNoteHTML(info);
    if (info) relabelCoreButtons(info);
  } catch (e) { /* detection is cosmetic */ }
  // A fresh sing-box also installs wintun.dll on Windows: the VPN tab's
  // preflight card must stop asking for what just arrived.
  refreshTunPreflight();
}

// One-button wintun.dll install (the Windows-only row next to the core
// buttons): the pinned official archive, sha256-verified, lands where
// sing-box reads it. The button carries the progress like updateCore.
async function installWintun() {
  if (S.coreUpd["wintun"]) return;
  S.coreUpd["wintun"] = true;
  const btn = $("btn-upd-wintun");
  const oldLabel = btn ? btn.textContent : "";
  if (btn) { btn.disabled = true; btn.textContent = t("toast.coreDl"); }
  try {
    const dir = await api().WintunInstall();
    showToast("✓ wintun.dll → " + dir);
  } catch (e) { showToast("✕ " + ((e && e.message) || e)); }
  S.coreUpd["wintun"] = false;
  if (btn) { btn.disabled = false; btn.textContent = oldLabel; }
  try {
    const info = await api().DetectCores();
    const note = $("core-note");
    if (note && info) note.innerHTML = coreNoteHTML(info);
  } catch (e) { /* detection is cosmetic */ }
  refreshTunPreflight();
}

function coreNoteHTML(info) {
  const fmt = (f, p) => (f ? "✓ " + p : "✕ not found");
  let out = `wisteria-client: ${esc(fmt(info.wisteria_found, info.wisteria_path))}<br>` +
    `sing-box: ${esc(fmt(info.singbox_found, info.singbox_path))}<br>` +
    `xray: ${esc(fmt(info.xray_found, info.xray_path))}`;
  if (info.os === "windows") {
    out += `<br>wintun.dll: ${esc(fmt(info.wintun_found, info.wintun_path))}`;
  }
  return out;
}

// When a core is not installed at all, the upstream button is a first
// download, not an update — say so ("Скачать xray") instead of the
// misleading "Обновить".
function relabelCoreButtons(info) {
  const b1 = $("btn-upd-singbox"), b2 = $("btn-upd-xray"), b3 = $("btn-upd-wisteria");
  if (b1) b1.textContent = (info.singbox_found ? t("set.updCore") : t("set.dlCore")) + " sing-box";
  if (b2) b2.textContent = (info.xray_found ? t("set.updCore") : t("set.dlCore")) + " xray";
  if (b3) b3.textContent = (info.wisteria_found ? t("set.updCore") : t("set.dlCore")) + " wisteria";
  // wintun.dll is a Windows-only install (sing-box's TUN helper); the
  // row stays hidden elsewhere.
  const bw = $("btn-upd-wintun");
  if (bw) bw.classList.toggle("hidden", info.os !== "windows");
}

// ---------- per-render event wiring ----------
function wireProfileList(scope) {
  scope.querySelectorAll(".server-list-item").forEach((row) => {
    row.addEventListener("click", async (ev) => {
      if (ev.target.closest(".sli-act")) return;
      if (S.busy) { showToast(t("toast.busy")); return; }
      S.selected = row.dataset.id;
      await api().SetActive(row.dataset.id);
      const wasConnected = S.state.state === "connected";
      renderContent();
      if (wasConnected) {
        S.busy = true;
        markConnecting(row.dataset.id);
        try { await withTimeout(api().Connect(row.dataset.id), 45000, t("toast.stuck")); }
        catch (e) {
          showToast((e && e.message) || e);
          try { renderState(await api().GetState()); } catch (_) {}
        }
        finally { S.busy = false; }
      }
    });
    row.addEventListener("dblclick", (ev) => {
      if (ev.target.closest(".sli-act")) return;
      switchTab("vpn");
    });
  });
  scope.querySelectorAll(".sli-act.shareb").forEach((b) => b.addEventListener("click", (ev) => {
    ev.stopPropagation();
    openQrModal(b.dataset.id);
  }));
  scope.querySelectorAll(".sli-act.editb").forEach((b) => b.addEventListener("click", (ev) => {
    ev.stopPropagation();
    openEditModal(b.dataset.id);
  }));
  scope.querySelectorAll(".sli-act.pingb").forEach((b) => b.addEventListener("click", async (ev) => {
    ev.stopPropagation();
    await api().PingProfile(b.dataset.id);
    await loadProfiles();
  }));
  scope.querySelectorAll(".sli-act.del").forEach((b) => b.addEventListener("click", async (ev) => {
    ev.stopPropagation();
    if (b.closest(".sub-head")) {
      if (!(await askConfirm(t("subs.confirmDelete")))) return;
      try { await api().DeleteSubscription(b.dataset.id); await loadSubs(); await loadProfiles(); }
      catch (e) { showToast((e && e.message) || e); }
      return;
    }
    if (!(await askConfirm(t("confirm.delete")))) return;
    try { await api().DeleteProfile(b.dataset.id); await loadProfiles(); }
    catch (e) { showToast((e && e.message) || e); }
  }));
  scope.querySelectorAll(".sub-head").forEach((h) => h.addEventListener("click", (ev) => {
    if (ev.target.closest(".sli-act")) return;
    toggleCollapsed(h.dataset.key);
  }));
  scope.querySelectorAll(".sub-refresh").forEach((b) => b.addEventListener("click", async (ev) => {
    ev.stopPropagation();
    await refreshSub(b.dataset.id);
  }));
}

function closeShareModal() { const m = $("modal-share"); if (m) m.classList.remove("show"); }
function closeEditModal() { const m = $("modal-edit"); if (m) m.classList.remove("show"); }

function openQrModal(id) {
  const p = S.profiles.find((x) => x.id === id);
  if (!p) return;
  if (!p.raw) { showToast(t("profiles.noRaw")); return; }
  try {
    const r = window.WisteriaQR.qrSvg(p.raw, { quiet: 4 });
    $("qr-wrap").innerHTML = r.svg;
    S.pendingShare = p.raw;
    $("modal-share").classList.add("show");
  } catch (e) { showToast((e && e.message) || String(e)); }
}

let editId = "";
function syncEditTlsRows() {
  const tls = !!$("e-transport") && $("e-transport").value === "tls";
  const sg = $("e-sni-group"), iw = $("e-insec-wrap");
  if (sg) sg.classList.toggle("hidden", !tls);
  if (iw) iw.classList.toggle("hidden", !tls);
}
function openEditModal(id) {
  const p = S.profiles.find((x) => x.id === id);
  if (!p) return;
  editId = id;
  $("e-name").value = p.name || "";
  $("e-host").value = p.host || "";
  $("e-port").value = p.port || "";
  $("e-transport").value = p.transport === "tls" ? "tls" : "tcp";
  $("e-sni").value = p.tls_sni || "";
  $("e-insecure").checked = !!p.tls_insecure;
  syncEditTlsRows();
  $("modal-edit").classList.add("show");
}
async function saveEditedProfile() {
  const payload = {
    id: editId,
    name: $("e-name").value.trim(),
    host: $("e-host").value.trim(),
    port: parseInt($("e-port").value, 10) || 0,
    transport: $("e-transport").value,
    tls_sni: $("e-sni").value.trim(),
    tls_insecure: $("e-insecure").checked,
  };
  try {
    await api().EditProfile(payload);
    closeEditModal();
    showToast(t("toast.saved"));
    await loadProfiles();
  } catch (e) { showToast((e && e.message) || e); }
}

async function runPingAll() {
  if (S.pfPingBusy) return;
  S.pfPingBusy = true;
  renderContent();
  try {
    await api().PingAllProfiles();
    showToast(t("profiles.pingAll") + " ✓");
  } catch (e) { showToast((e && e.message) || e); }
  finally {
    S.pfPingBusy = false;
    await loadProfiles();
  }
}

async function pickFastest() {
  const candidates = S.profiles.filter(matchProfile).filter((p) => Number(p.latency_ms) > 0);
  if (!candidates.length) { showToast(t("profiles.fastestNone")); return; }
  candidates.sort((a, b) => Number(a.latency_ms) - Number(b.latency_ms));
  const best = candidates[0];
  const wasConnected = S.state.state === "connected";
  try {
    S.selected = best.id;
    await api().SetActive(best.id);
    if (wasConnected) {
      S.busy = true;
      markConnecting(best.id);
      await withTimeout(api().Connect(best.id), 45000, t("toast.stuck"));
      S.busy = false;
    }
    showToast(t("profiles.fastestDone").replace("{name}", best.name).replace("{ms}", String(best.latency_ms)));
    await loadProfiles();
  } catch (e) {
    S.busy = false;
    showToast((e && e.message) || e);
  }
}

function wireContent() {
  const on = (id, ev, fn) => { const el = $(id); if (el) el.addEventListener(ev, fn); };

  on("btn-toggle", "click", async () => {
    // A connecting/reconnecting attempt is cancellable: the button says
    // "Отмена", and this click must not be swallowed by the busy guard the
    // in-flight Connect call holds for up to 45 seconds.
    if (S.state.state === "connecting" || S.state.reconnecting) {
      buzz(12);
      S.busy = false;
      try { await api().Disconnect(); } catch (e) { showToast((e && e.message) || e); }
      return;
    }
    if (S.state.state === "connected") {
      buzz(12);
      try { await api().Disconnect(); } catch (e) { showToast((e && e.message) || e); }
      return;
    }
    if (S.busy) { showToast(t("toast.busy")); return; }
    if (!S.selected) { openModal(); return; }
    try {
      buzz(10);
      S.busy = true;
      markConnecting(S.selected);
      await withTimeout(api().Connect(S.selected), 45000, t("toast.stuck"));
    } catch (e) {
      showToast((e && e.message) || e);
      // markConnecting drew a local "connecting" screen; a fast backend
      // failure returns with no state event of its own — re-pull the real
      // state so the UI never stays wedged until a restart.
      try { renderState(await api().GetState()); } catch (_) { /* keep local view */ }
    }
    finally { S.busy = false; }
  });
  on("btn-err-logs", "click", () => switchTab("logs"));
  on("card-profile", "click", () => switchTab("profiles"));

  on("btn-add", "click", openModal);
  wireProfileList($("pf-list") || document);

  on("pf-search", "input", (ev) => {
    S.pfFilter = ev.target.value;
    const box = $("pf-list");
    if (box) { box.innerHTML = renderProfilesList(); wireProfileList(box); }
  });
  on("pf-sort", "change", (ev) => {
    S.pfSort = ev.target.value;
    const box = $("pf-list");
    if (box) { box.innerHTML = renderProfilesList(); wireProfileList(box); }
  });
  on("btn-ping-all", "click", runPingAll);
  on("btn-fastest", "click", pickFastest);

  on("btn-clip", "click", async () => {
    try {
      const r = await api().ImportFromClipboard();
      await loadSubs();
      await loadProfiles();
      const parts = [];
      if (r.subscriptions) parts.push(t("toast.clipSubs").replace("{n}", r.subscriptions));
      if (r.added && r.added.length) parts.push(t("toast.clipProfiles").replace("{n}", r.added.length));
      if (r.errors && r.errors.length) {
        const fe = r.errors[0];
        parts.push("✕ " + (fe.line ? fe.line + ": " : "") + fe.error);
      }
      showToast(parts.length ? parts.join(" · ") : t("toast.clipNone"));
    } catch (e) { showToast((e && e.message) || e); }
  });

  on("btn-clear-logs", "click", async () => {
    $("log-view").textContent = "";
    try { if (api().ClearLogs) await api().ClearLogs(); } catch (e) {}
  });
  // Copy is for support: the log alone says nothing about the build or the
  // state, so prepend a small header with both.
  on("btn-copy-logs", "click", async () => {
    const view = $("log-view");
    const body = view ? view.innerText.replace(/\s+$/, "") : "";
    if (!body) { showToast(t("logs.copyEmpty")); return; }
    const st = S.state || {};
    const head = "Wisteria " + (S.version || "") + " · " + (navigator.userAgent || "") + "\n" +
                 new Date().toISOString() + " state=" + (st.state || "?") +
                 (st.endpoint ? " endpoint=" + st.endpoint : "") + "\n--- logs ---\n";
    const text = head + body;
    const n = body.split("\n").length;
    try {
      await api().CopyToClipboard(text);
      showToast(t("logs.copied").replace("{n}", String(n)));
    } catch (e) {
      try { await navigator.clipboard.writeText(text); showToast(t("logs.copied").replace("{n}", String(n))); }
      catch (_) { showToast(t("logs.copyFail")); }
    }
  });

  document.querySelectorAll(".theme-option").forEach((opt) => opt.addEventListener("click", async () => {
    S.theme = opt.dataset.theme;
    applyTheme();
    await saveSettings();
    if (S.tab === "settings") { renderContent(); fillSettingsForm(S.cfg); }
    else document.querySelectorAll(".theme-option").forEach((o) => o.classList.toggle("selected", o === opt));
  }));
  on("btn-checkip", "click", async () => {
    try {
      const ip = await api().GetExternalIP();
      showToast("IP: " + ip);
    } catch (e) {
      const msg = (e && e.message) || String(e);
      // A reconnect is not a failure: the core has no live session yet and the
      // SOCKS layer reports that as "host unreachable".
      showToast(msg.indexOf("туннель переподключается") >= 0
        ? t("toast.ipRetry")
        : "✕ " + t("toast.ipFail") + ": " + msg);
    }
  });
  document.querySelectorAll("[data-i18n]").forEach(applyI18nEl);

  on("btn-detect", "click", async () => {
    const info = await api().DetectCores();
    $("core-note").innerHTML = coreNoteHTML(info);
    relabelCoreButtons(info);
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
  on("btn-upd-wintun", "click", installWintun);
  on("btn-upd-xray", "click", () => updateCore("xray"));
  on("btn-upd-wisteria", "click", () => updateCore("wisteria"));
  // Cores tab: label the buttons by what is actually installed.
  if ($("btn-upd-singbox") || $("btn-upd-xray")) {
    api().DetectCores().then((info) => {
      if (info) relabelCoreButtons(info);
    }).catch(() => { /* detection is cosmetic here */ });
  }
  on("btn-open-guide", "click", () => openGuideModal());
  on("btn-open-guide-2", "click", () => openGuideModal());
  on("btn-copy-hwid", "click", async () => {
    const id = S.devInfo && S.devInfo.id;
    if (!id) return;
    try { await api().CopyToClipboard(id); showToast(t("toast.copied")); }
    catch (e) { try { await navigator.clipboard.writeText(id); showToast(t("toast.copied")); } catch (_) {} }
  });
  on("btn-shortcuts", "click", async () => {
    try {
      await api().InstallShortcuts();
      showToast(t("toast.shortcutsOk"));
    } catch (e) { showToast((e && e.message) || e); }
  });
  document.querySelectorAll(".set-tab").forEach((b) => b.addEventListener("click", async () => {
    if (b.dataset.settab === S.setTab) return;
    try { await saveSettings(true); } catch (e) {}
    S.setTab = b.dataset.settab;
    renderContent();
  }));
  on("btn-save-settings", "click", saveSettings);
}

function applyI18nEl(el) {
  const k = el.dataset.i18n;
  if (!k) return;
  el.textContent = t(k);
}

// ---------- settings ----------
async function loadSettings() {
  const s = await api().GetSettings();
  S.cfg = s;
  S.theme = s.theme || "lilac";
  S.customTheme = Object.assign({}, DEFAULT_CUSTOM, s.custom_theme || {});
  S.uiScale = s.ui_scale || 100;
  lang = s.language || "ru";
  applyTheme();
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
  setv("s-autoping", s.auto_ping_sec || 0);
  setc("s-listenall", s.listen_all);
  setc("s-bypasslan", s.bypass_lan);
  setc("s-bypassru", !s.no_bypass_ru); // checkbox states the feature, the flag states the opt-out
  setv("s-remotedns", s.remote_dns || "");
  setv("s-loglevel", s.log_level || "warn");
  setv("s-memlimit", s.mem_limit_mb || 0);
  setc("s-tun", s.tun_enabled);
  setv("s-tunstack", s.tun_stack || "mixed");
  setc("s-tunstrict", s.tun_strict_route);
  setv("s-route-direct", s.route_direct || "");
  setv("s-route-block", s.route_block || "");
  setv("s-core-ss", s.core_ss || "xray");
  setv("s-core-vmess", s.core_vmess || "xray");
  setv("s-core-vless", s.core_vless || "xray");
  setv("s-core-trojan", s.core_trojan || "xray");
  setv("s-xray", s.xray_core || "");
  setv("s-ghtoken", s.github_token || "");
  setv("s-scale", s.ui_scale || 100);
  setv("s-wpreset", s.wisteria_preset || "stealth");
  const sv = $("s-scale-val"); if (sv) sv.textContent = (s.ui_scale || 100) + "%";
  const ct = Object.assign({}, DEFAULT_CUSTOM, s.custom_theme || {});
  ["accent", "bg", "card", "fg", "muted"].forEach((k) => setv("ct-" + k, ct[k]));
  const sel = $("s-autoconnect");
  if (sel) {
    sel.innerHTML = `<option value="">—</option>` +
      (S.profiles.length ? `<option value="__auto__">${esc(t("auto.name"))} (AUTO)</option>` : "") +
      S.profiles.map((p) => `<option value="${p.id}">${esc(p.name)} (${esc(p.type)})</option>`).join("");
    sel.value = s.auto_connect || "";
  }
  if (!S.devInfo && api().GetDeviceInfo) {
    api().GetDeviceInfo().then((d) => {
      S.devInfo = d;
      const el = $("s-hwid");
      if (el && d && d.id) el.textContent = d.id;
    }).catch(() => {});
  }
  if (!S.coreInfo && api().DetectCores) {
    api().DetectCores().then((ci) => {
      const had = !!S.coreInfo;
      S.coreInfo = ci;
      if (!had && S.tab === "settings" && S.setTab === "general") renderContent();
    }).catch(() => {});
  }
}

async function saveSettings(silent) {
  const cur = await api().GetSettings();
  const g = (id) => { const el = $(id); return el ? el : null; };
  const num = (id, def) => { const v = parseInt($(id).value, 10); return Number.isFinite(v) ? v : def; };
  const next = {
    language: lang,
    theme: S.theme,
    ui_scale: g("s-scale") ? num("s-scale", 100) : cur.ui_scale,
    wisteria_preset: g("s-wpreset") ? $("s-wpreset").value : cur.wisteria_preset,
    custom_theme: S.customTheme || cur.custom_theme,
    local_port: g("s-port") ? num("s-port", 2080) : cur.local_port,
    http_port: g("s-http-port") ? num("s-http-port", 2081) : cur.http_port,
    sys_proxy: g("s-sysproxy") ? $("s-sysproxy").checked : cur.sys_proxy,
    bridge_enabled: g("s-bridge") ? $("s-bridge").checked : cur.bridge_enabled,
    auto_connect: g("s-autoconnect") ? $("s-autoconnect").value : cur.auto_connect,
    wisteria_core: g("s-wisteria") ? $("s-wisteria").value.trim() : cur.wisteria_core,
    singbox_core: g("s-singbox") ? $("s-singbox").value.trim() : cur.singbox_core,
    start_minimized: g("s-startmin") ? $("s-startmin").checked : cur.start_minimized,
    confirm_exit: g("s-confirmexit") ? $("s-confirmexit").checked : cur.confirm_exit,
    no_update_check: g("s-updcheck") ? !$("s-updcheck").checked : cur.no_update_check,
    github_token: g("s-ghtoken") ? $("s-ghtoken").value.trim() : cur.github_token,
    auto_ping_sec: g("s-autoping") ? Math.max(0, num("s-autoping", 0)) : cur.auto_ping_sec,
    listen_all: g("s-listenall") ? $("s-listenall").checked : cur.listen_all,
    bypass_lan: g("s-bypasslan") ? $("s-bypasslan").checked : cur.bypass_lan,
    no_bypass_ru: g("s-bypassru") ? !$("s-bypassru").checked : cur.no_bypass_ru,
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
  S.cfg = await api().GetSettings();
  if (!silent) showToast(t("toast.saved"));
}

function hexToRgb(hex) {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex || "");
  if (!m) return "0, 232, 162";
  const n = parseInt(m[1], 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255].join(", ");
}

function applyScale() {
  const app = $("app");
  if (!app) return;
  const z = (S.uiScale || 100) / 100;
  app.style.zoom = "";
  app.style.height = "";
  app.style.width = "";
  app.style.setProperty("--ui-zoom", String(z));
}

function applyTheme() {
  const app = $("app");
  const inline = ["--accent", "--accent-rgb", "--card", "--fg", "--muted", "--border", "--bg", "--surface"];
  if (S.theme === "custom") {
    const c = Object.assign({}, DEFAULT_CUSTOM, S.customTheme || {});
    app.className = "theme-custom";
    app.style.background = c.bg;
    app.style.color = c.fg;
    app.style.setProperty("--accent", c.accent);
    app.style.setProperty("--accent-rgb", hexToRgb(c.accent));
    app.style.setProperty("--bg", c.bg);
    app.style.setProperty("--surface", c.card);
    app.style.setProperty("--card", c.card);
    app.style.setProperty("--fg", c.fg);
    app.style.setProperty("--muted", c.muted);
    app.style.setProperty("--border", "rgba(" + hexToRgb(c.fg) + ", 0.08)");
  } else {
    app.className = "theme-" + S.theme;
    app.style.background = "";
    app.style.color = "";
    inline.forEach((p) => app.style.removeProperty(p));
  }
  applyScale();
}

// ---------- connect feedback (Web Animations API) ----------
// CSS keyframes alone proved unreliable on Windows/WebView2 (the button just
// lit up with no sweep). WAAPI runs on the compositor in both Chromium and
// WebKitGTK and is not suppressed by the OS reduce-motion setting, so the
// one-shot "tunnel is up" confirmation plays everywhere. Ambient loops stay
// CSS and stay gated by prefers-reduced-motion.
let spinAnim = null;
let spinStart = 0;    // performance.now() when the current lap began
let connTimer = null; // pending "finish the lap" connected reveal
let sawConnecting = false; // a connecting state was observed this page session
const SPIN_MS = 1100; // one full arc revolution
function setSpin(on) {
  const arc = document.querySelector(".connect-btn .spin-arc");
  if (on) {
    if (spinAnim || !arc) return; // idempotent: never restart a running lap
    spinStart = performance.now();
    spinAnim = arc.animate([{ transform: "rotate(0deg)" }, { transform: "rotate(360deg)" }], { duration: SPIN_MS, iterations: Infinity });
  } else {
    if (spinAnim) { spinAnim.cancel(); spinAnim = null; }
    spinStart = 0;
  }
}
function playConnectFx(btn) {
  try {
    btn.animate([{ transform: "scale(.93)" }, { transform: "scale(1.05)", offset: .6 }, { transform: "scale(1)" }], { duration: 520, easing: "cubic-bezier(.34,1.56,.64,1)" });
    const arc = btn.querySelector(".spin-arc");
    if (arc) arc.animate([{ transform: "rotate(0deg)", opacity: 1 }, { transform: "rotate(360deg)", opacity: 0 }], { duration: 700, easing: "ease-out" });
    // The rings are driven by .connected .ring-anim's CSS pulse; a WAAPI
    // animation here competed with it and froze the pulse mid-lap.
  } catch (e) { /* WAAPI unavailable: CSS still shows the state */ }
}

// The connected reveal waits for the arc to finish exactly one full
// revolution: a sub-second connect used to swap power→check after a quarter
// lap, and a structural rebuild (sys_proxy/tun flip at connect on Windows)
// recreated the button born-connected — no crossfade, no fx at all.
function beginConnectedLap() {
  const btn = $("btn-toggle");
  if (!btn) return;
  if (!sawConnecting) {
    // Boot reconciliation, not a fresh connect: no fake connecting lap —
    // start from the power glyph and let the crossfade play on the live node.
    btn.classList.remove("connected");
    requestAnimationFrame(() => requestAnimationFrame(() => revealConnected()));
    return;
  }
  btn.classList.remove("connected");
  btn.classList.add("connecting");
  btn.disabled = true;
  const txt = btn.querySelector(".connect-text");
  if (txt) txt.textContent = t("vpn.connecting");
  const labelEl = document.querySelector(".vpn-status .label");
  if (labelEl) labelEl.textContent = t("vpn.connecting");
  const statusEl = document.querySelector(".vpn-status");
  if (statusEl) { statusEl.classList.remove("on"); statusEl.classList.add("mid"); }
  setSpin(true);
  const elapsed = spinStart ? performance.now() - spinStart : 0;
  const wait = Math.max(0, SPIN_MS - elapsed);
  if (connTimer) clearTimeout(connTimer);
  connTimer = setTimeout(() => { connTimer = null; revealConnected(); }, wait);
}

function revealConnected() {
  const btn = $("btn-toggle");
  if (!btn || S.state.state !== "connected") return;
  if (btn.classList.contains("connected")) return; // already revealed
  const labelEl = document.querySelector(".vpn-status .label");
  if (labelEl) labelEl.textContent = t("vpn.protected");
  const statusEl = document.querySelector(".vpn-status");
  if (statusEl) { statusEl.classList.add("on"); statusEl.classList.remove("mid"); }
  btn.classList.remove("connecting");
  btn.classList.add("connected");
  btn.disabled = false;
  const txt = btn.querySelector(".connect-text");
  if (txt) txt.textContent = t("vpn.connected");
  setSpin(false);
  buzz(20);
  playConnectFx(btn);
  renderSidebarFooter();
}

// Live previews (look tab scale + custom palette) and the VPN-tab TUN switch.
// Delegated at module scope so they survive re-renders without duplicating.
function isDesktopSettingsControl(el) {
  if (!el || !el.id || !/^s-/.test(el.id) || el.id === "s-lang" || el.id === "s-scale") return false;
  const wrap = $("content-wrapper");
  return !!wrap && wrap.contains(el);
}
document.addEventListener("input", (ev) => {
  const id = ev.target.id;
  if (id === "s-scale") {
    // Do NOT mutate app.style.zoom mid-drag: changing zoom on an ancestor
    // shifts pointer coordinates relative to the range track and breaks dragging.
    S.uiScale = parseInt(ev.target.value, 10) || 100;
    const lv = $("s-scale-val"); if (lv) lv.textContent = S.uiScale + "%";
  } else if (id && id.indexOf("ct-") === 0) {
    S.customTheme = Object.assign({}, DEFAULT_CUSTOM, S.customTheme || {});
    S.customTheme[id.slice(3)] = ev.target.value;
    applyTheme();
    scheduleLookSave();
  } else if (isDesktopSettingsControl(ev.target)) {
    scheduleLookSave();
  }
});

// Live previews persist on their own: dragging a color or the scale used to
// reset on restart unless the user happened to press Save afterwards.
let lookSaveTimer = null;
function scheduleLookSave() {
  clearTimeout(lookSaveTimer);
  lookSaveTimer = setTimeout(() => { saveSettings(true); }, 700);
}
document.addEventListener("change", async (ev) => {
  if (ev.target.id === "s-scale") {
    S.uiScale = parseInt(ev.target.value, 10) || 100;
    const lv = $("s-scale-val"); if (lv) lv.textContent = S.uiScale + "%";
    applyScale();
    scheduleLookSave();
    return;
  }
  if (isDesktopSettingsControl(ev.target)) {
    scheduleLookSave();
    return;
  }
  if (ev.target.id === "vpn-preset") {
    const pid = S.state.profile_id || S.selected;
    if (!pid) return;
    await api().SetProfilePreset(pid, ev.target.value);
    await loadProfiles();
    renderContent();
    return;
  }
  if (ev.target.name !== "mode") return;
  // The segmented control replaces the old TUN checkbox: proxy = system
  // proxy, tun = the sing-box helper over the core's SOCKS. The backend
  // applies a changed tun_enabled immediately (it reconnects a running
  // session), which is exactly why a non-ready TUN must be refused below.
  // The thumb moves via data-active without a re-render — focus stays on
  // the control.
  const target = ev.target.value;
  const track = $("seg-track") || ev.target.closest(".seg-track");
  if (target === "tun" && S.state.state === "connected") {
    // Switching mode is saved for the NEXT connect, but a reconnect (the
    // server drops, the user hits reconnect) would apply it immediately.
    // Turning a working proxy into a TUN session that cannot start is not
    // a "mode preference": keep the working session and say what is
    // missing first.
    const pf = await refreshTunPreflight();
    if (!pf || !pf.ready) {
      S.tunStatus = pf;
      if (track) track.dataset.active = "proxy";
      const proxy = document.querySelector('input[name="mode"][value="proxy"]');
      if (proxy) proxy.checked = true;
      showToast(t("tun.notReady").replace("{why}", tunMissingShort(pf)));
      return;
    }
  }
  const cur = await api().GetSettings();
  cur.tun_enabled = target === "tun";
  await api().SaveSettings(cur);
  S.cfg = cur;
  if (target === "tun") {
    if (S.state.state !== "connected") {
      // Re-render so the preflight card (with its one-click setup) appears
      // right away; the connect button is idle here, so a rebuild is safe.
      renderContent();
      await refreshTunPreflight();
    } else if (track) {
      track.dataset.active = "tun";
    }
  } else if (track) {
    track.dataset.active = "proxy";
  }
  return;
});

// ---------- profiles ----------
async function loadProfiles() {
  S.profiles = (await api().ListProfiles()) || [];
  // A failed state read must not abort the boot chain into a blank screen.
  let st = null;
  try { st = await api().GetState(); } catch (e) { /* fall back to active id */ }
  if (st && st.profile_id) S.selected = st.profile_id;
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
    // The core prefixes its own lines with a timestamp; the view adds one
    // from l.at — strip the embedded one so every line reads once.
    const text = String(l.text).replace(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} /, "").replace(/^\d{2}:\d{2}:\d{2} /, "");
    div.textContent = `${new Date(l.at).toLocaleTimeString()} [${l.tag}] ${text}\n`;
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
  if (document.hidden || S.tab !== "vpn" || S.state.state !== "connected") return;
  const el = $("stat-uptime");
  if (!el) return;
  const html = uptimeHTML(S.state.started_at);
  if (html) el.innerHTML = html;
}, 1000);

function renderState(st) {
  const prev = S.state;
  S.state = st || S.state;
  if (S.state.state === "connecting" || S.state.reconnecting) sawConnecting = true;
  if (S.state.error && !(prev && prev.error)) buzz([40, 60, 40]); // a failure surfaced
  if (S.tab !== "vpn") { renderSidebarFooter(); return; }
  // Rebuild only on structural changes (profile switch, error box).
  // Same-profile transitions (idle → connecting → connected) patch the DOM
  // in place: an innerHTML swap would recreate the button node and restart
  // its CSS animation from frame zero, which reads as the button animation
  // stuttering on every state event.
  const structural = !prev ||
    prev.profile_id !== S.state.profile_id ||
    (prev.error || "") !== (S.state.error || "");
  if (structural) {
    const prevConn = !!prev && prev.state === "connected";
    if (connTimer) { clearTimeout(connTimer); connTimer = null; }
    setSpin(false); // the rebuild replaces the arc node the spin targets
    renderContent();
    if (S.state.state === "connected" && !prevConn) beginConnectedLap();
    else setSpin(S.state.state === "connecting" || !!S.state.reconnecting);
    return;
  }
  patchVpnState();
  renderSidebarFooter();
}

function patchVpnState() {
  const st = S.state;
  const conn = st.state === "connected";
  const connecting = st.state === "connecting";
  const reconn = !!st.reconnecting;
  const labelEl = document.querySelector(".vpn-status .label");
  if (labelEl) labelEl.textContent = conn ? t("vpn.protected") : reconn ? t("vpn.reconnecting") + (st.attempt ? " " + t("vpn.attempt").replace("{n}", st.attempt) : "") : connecting ? t("vpn.connecting") : t("vpn.unprotected");
  const statusEl = document.querySelector(".vpn-status");
  if (statusEl) {
    statusEl.classList.toggle("on", conn);
    statusEl.classList.toggle("mid", connecting || reconn);
  }
  const btn = $("btn-toggle");
  if (btn) {
    const wasConn = btn.classList.contains("connected");
    if (conn && !wasConn) { beginConnectedLap(); return; }
    if (!conn && connTimer) { clearTimeout(connTimer); connTimer = null; }
    btn.classList.toggle("connected", conn);
    btn.classList.toggle("connecting", connecting || reconn);
    // Never disable while connecting: the same button cancels the attempt.
    btn.disabled = false;
    const txt = btn.querySelector(".connect-text");
    if (txt) txt.textContent = conn ? t("vpn.connected") : (connecting || reconn) ? t("vpn.cancel") : t("vpn.disconnected");
    setSpin(connecting || reconn);
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
document.querySelectorAll("#modal-share, #modal-edit").forEach((ov) => {
  ov.addEventListener("click", (ev) => { if (ev.target === ov) { closeShareModal(); closeEditModal(); } });
});
if ($("qr-close")) $("qr-close").addEventListener("click", closeShareModal);
if ($("edit-close")) $("edit-close").addEventListener("click", closeEditModal);
if ($("e-transport")) $("e-transport").addEventListener("change", syncEditTlsRows);
if ($("btn-edit-save")) $("btn-edit-save").addEventListener("click", saveEditedProfile);
if ($("btn-qr-copy")) $("btn-qr-copy").addEventListener("click", async () => {
  try { await api().CopyToClipboard(S.pendingShare); showToast(t("toast.copied")); }
  catch (e) { showToast((e && e.message) || e); }
});
document.addEventListener("keydown", (ev) => {
  if (ev.key !== "Escape") return;
  if ($("modal-add") && $("modal-add").classList.contains("show")) closeModal();
  else if ($("modal-share") && $("modal-share").classList.contains("show")) closeShareModal();
  else if ($("modal-edit") && $("modal-edit").classList.contains("show")) closeEditModal();
  else if ($("modal-update") && $("modal-update").classList.contains("show")) closeUpdateModal();
  else if ($("modal-guide") && $("modal-guide").classList.contains("show")) closeGuideModal();
});

// ---------- Update & Guide Modals ----------
function openUpdateModal(info) {
  const ov = $("modal-update");
  if (!ov || !info) return;
  const verEl = $("upd-modal-ver");
  if (verEl) verEl.textContent = "v" + (info.current || S.version || "?") + " → v" + (info.latest || "?");
  const goBtn = $("upd-modal-go");
  if (goBtn) {
    goBtn.disabled = !!S.updBusy;
    goBtn.textContent = S.updReady ? t("set.updRestart") : (S.updBusy ? t("toast.coreDl") : t("set.updApply"));
  }
  ov.classList.add("show");
}
function closeUpdateModal() {
  const ov = $("modal-update");
  if (ov) ov.classList.remove("show");
}

const GUIDE_SECTIONS = [
  ["cores", "guide.tabCores"],
  ["updates", "guide.tabUpdates"],
  ["features", "guide.tabFeatures"],
  ["faq", "guide.tabFaq"],
];

function renderGuideBody(tab) {
  if (lang === "en") {
    if (tab === "cores") return `
      <h4>1. What cores are used?</h4>
      <ul>
        <li><code>wisteria-client</code> — native anti-DPI protocol core (Noise + obfuscation + yamux).</li>
        <li><code>sing-box</code> — universal proxy core for VLESS, VMess, Shadowsocks, Trojan, Hysteria2, TUIC, and system-wide TUN mode.</li>
        <li><code>xray</code> — optional alternative engine for VLESS (REALITY/XTLS), VMess, Trojan, and Shadowsocks.</li>
      </ul>
      <h4>2. Automatic installation</h4>
      <p>Open <b>Settings → Cores</b> and click <b>Download / Update</b> next to <code>sing-box</code>, <code>xray</code>, or <code>wisteria</code>. On Windows, downloading <code>sing-box</code> also installs <code>wintun.dll</code> automatically.</p>
      <h4>3. TUN mode permissions</h4>
      <ul>
        <li><b>Windows:</b> run the app as Administrator (use the button in Settings → Connection).</li>
        <li><b>Linux:</b> grant network capability once: <code>sudo setcap cap_net_admin=+ep /path/to/sing-box</code>.</li>
      </ul>`;
    if (tab === "updates") return `
      <h4>App updates</h4>
      <p>Wisteria checks GitHub Releases on startup and shows a popup when a newer version is published. Click <b>Download and install</b> — the binary is verified via SHA-256 and replaced atomically, then restarted.</p>
      <h4>Core updates</h4>
      <p>Go to <b>Settings → Cores</b> to check or update <code>sing-box</code> and <code>xray</code> directly from their official upstream releases without touching your profiles.</p>`;
    if (tab === "features") return `
      <h4>Auto-Profile (Failover)</h4>
      <p>Select <b>Auto-Profile</b> at the top of the Profiles list. Wisteria picks the lowest-latency server and automatically switches to another working profile if the active tunnel drops.</p>
      <h4>TUN & Bypass RU</h4>
      <p>Enable <b>TUN mode</b> on the main screen to route all system traffic (including games and non-proxy apps). Russian domains/IPs bypass the tunnel by default unless disabled in Settings → Routing.</p>
      <h4>Hardware Device ID (HWID)</h4>
      <p>Each device sends a deterministic hardware fingerprint (<code>X-Device-ID</code>) when refreshing subscriptions so panels can enforce per-link device limits.</p>`;
    return `
      <h4>Why does a link fail to connect?</h4>
      <p>Open <b>Diagnostics</b> (for Wisteria profiles) or <b>Logs</b>. Common causes: expired/revoked key, wrong clock on the device, or missing core binary in <b>Settings → Cores</b>.</p>
      <h4>How do I share a profile?</h4>
      <p>Click the share icon on any imported link profile to display its QR code or copy the raw share link.</p>`;
  }
  if (tab === "cores") return `
    <h4>1. Какие ядра используются</h4>
    <ul>
      <li><code>wisteria-client</code> — нативное ядро протокола Wisteria (anti-DPI: Noise + обфускация + yamux).</li>
      <li><code>sing-box</code> — ядро для VLESS, VMess, Shadowsocks, Trojan, Hysteria2, TUIC и системного режима TUN.</li>
      <li><code>xray</code> — альтернативный движок для VLESS (REALITY/XTLS), VMess, Trojan и Shadowsocks.</li>
    </ul>
    <h4>2. Установка в один клик</h4>
    <p>Перейдите в <b>Настройки → Ядра</b> и нажмите <b>Скачать / Обновить</b> рядом с нужным ядром. На Windows вместе с <code>sing-box</code> автоматически скачивается <code>wintun.dll</code>.</p>
    <h4>3. Права для режима TUN</h4>
    <ul>
      <li><b>Windows:</b> запустите приложение от имени администратора (кнопка в Настройки → Подключение).</li>
      <li><b>Linux:</b> выдайте права ядру одной командой: <code>sudo setcap cap_net_admin=+ep /путь/к/sing-box</code>.</li>
    </ul>`;
  if (tab === "updates") return `
    <h4>Обновление приложения</h4>
    <p>При запуске Wisteria проверяет свежие релизы и показывает всплывающее окно при наличии новой версии. Нажмите <b>Скачать и установить</b> — файл проверится по SHA-256, заменится автоматически и перезапустится.</p>
    <h4>Обновление ядер</h4>
    <p>В разделе <b>Настройки → Ядра</b> можно в любой момент обновить <code>sing-box</code> и <code>xray</code> из официальных релизов без потери профилей и настроек.</p>`;
  if (tab === "features") return `
    <h4>Автопрофиль (Автопереключение)</h4>
    <p>Выберите карточку <b>Автопрофиль</b> вверху списка профилей. Клиент сам выберет сервер с минимальным пингом и при обрыве связи автоматически переключится на следующий рабочий профиль.</p>
    <h4>Режим TUN и обход РФ</h4>
    <p>Переключатель <b>Системный прокси / TUN</b> на главном экране переводит весь трафик системы в туннель. Российские сайты и подсети по умолчанию идут напрямую (настраивается в Настройки → Маршрутизация).</p>
    <h4>Идентификатор устройства (HWID)</h4>
    <p>При обновлении подписок приложение передаёт аппаратный отпечаток устройства (<code>X-Device-ID</code>), чтобы панель могла контролировать лимит устройств на одну ссылку.</p>`;
  return `
    <h4>Профиль не подключается — что проверить?</h4>
    <p>Откройте вкладку <b>Диагностика</b> или <b>Логи</b>. Чаще всего причина в отозванном ключе, неточном системном времени или нескачанном ядре в <b>Настройки → Ядра</b>.</p>
    <h4>Как перенести профиль на телефон?</h4>
    <p>Нажмите иконку «Поделиться» в строке профиля — откроется QR-код, который можно отсканировать камерой в Wisteria Android.</p>`;
}

function openGuideModal(tab) {
  if (tab) S.guideTab = tab;
  const ov = $("modal-guide");
  if (!ov) return;
  const tabsEl = $("guide-tabs");
  if (tabsEl) {
    tabsEl.innerHTML = GUIDE_SECTIONS.map(([id, k]) =>
      `<button class="modal-tab ${S.guideTab === id ? "active" : ""}" data-gtab="${id}">${esc(t(k))}</button>`
    ).join("");
    tabsEl.querySelectorAll("[data-gtab]").forEach((b) => b.addEventListener("click", () => {
      openGuideModal(b.dataset.gtab);
    }));
  }
  const bodyEl = $("guide-body");
  if (bodyEl) bodyEl.innerHTML = renderGuideBody(S.guideTab);
  ov.classList.add("show");
}
function closeGuideModal() {
  const ov = $("modal-guide");
  if (ov) ov.classList.remove("show");
}

if ($("upd-modal-later")) $("upd-modal-later").addEventListener("click", closeUpdateModal);
if ($("modal-update")) $("modal-update").addEventListener("click", (ev) => { if (ev.target === $("modal-update")) closeUpdateModal(); });
if ($("upd-modal-go")) $("upd-modal-go").addEventListener("click", async () => {
  if (S.updReady) {
    try { await api().RestartApp(); } catch (e) { showToast((e && e.message) || e); }
    return;
  }
  if (S.updBusy) return;
  S.updBusy = true;
  openUpdateModal(S.upd);
  try {
    await api().ApplyAppUpdate();
    S.updReady = true;
    showToast(t("toast.updReady"));
  } catch (e) { showToast("✕ " + ((e && e.message) || e)); }
  S.updBusy = false;
  openUpdateModal(S.upd);
  if (S.tab === "settings") renderContent();
});
if ($("guide-close")) $("guide-close").addEventListener("click", closeGuideModal);
if ($("modal-guide")) $("modal-guide").addEventListener("click", (ev) => { if (ev.target === $("modal-guide")) closeGuideModal(); });


// manual form extra fields
function renderExtraFields() {
  const tp = $("m-type").value;
  const box = $("m-extra");
  box.innerHTML = "";
  (EXTRA[tp] || []).forEach(([key, label]) => {
    box.insertAdjacentHTML("beforeend",
      `<div class="input-group"><label>${esc(label)}</label><input type="text" data-field="${esc(key)}"/></div>`);
  });
  // Wisteria TLS transport (wave 4): the dedicated block replaces the
  // generic text inputs — a select plus two rows that only make sense on TLS.
  if (tp === "wisteria") {
    box.insertAdjacentHTML("beforeend",
      `<div class="input-group"><label>${esc(t("vpn.preset"))}</label><select data-field="preset"><option value="">—</option><option value="fast">fast</option><option value="balance">balance</option><option value="stealth">stealth</option></select></div>`);
  }
  const wst = $("m-wst-tls");
  if (wst) wst.classList.toggle("hidden", tp !== "wisteria");
  syncTlsRows();
}
// SNI + insecure checkbox appear only when transport = TLS.
function syncTlsRows() {
  const tls = !!$("m-transport") && $("m-transport").value === "tls";
  [["m-tls-sni-group"], ["m-tls-insec-wrap"]].forEach(([id]) => {
    const el = $(id);
    if (el) el.classList.toggle("hidden", !tls);
  });
}
$("m-type").addEventListener("change", renderExtraFields);
$("m-transport").addEventListener("change", syncTlsRows);

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
  document.querySelectorAll("#m-extra [data-field]").forEach((i) => { if (i.value.trim()) fields[i.dataset.field] = i.value.trim(); });
  const payload = {
    type: $("m-type").value, name: $("m-name").value.trim(),
    host: $("m-host").value.trim(), port: parseInt($("m-port").value, 10) || 0,
    fields,
  };
  // Wisteria TLS transport: the select sends "tcp"/"tls" (the backend
  // normalizes tcp onto the empty default); the SNI and the insecure
  // checkbox only carry meaning with TLS, so they ride only then.
  if (payload.type === "wisteria" && $("m-transport")) {
    payload.transport = $("m-transport").value;
    if (payload.transport === "tls") {
      payload.tls_sni = $("m-tls-sni").value.trim();
      payload.tls_insecure = $("m-tls-insec").checked;
    }
  }
  try {
    const p = await api().AddManual(payload);
    $("m-name").value = ""; $("m-host").value = ""; $("m-port").value = "";
    $("m-tls-sni").value = ""; $("m-tls-insec").checked = false; $("m-transport").value = "tcp";
    syncTlsRows();
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
    if (info && info.has_update && !S.updModalShown) {
      S.updModalShown = true;
      openUpdateModal(info);
    } else if (info && info.has_update) {
      showToast(t("set.updAvail") + ": v" + info.latest);
    }
    if (S.tab === "settings") renderContent();
  });
  // «Открыть логи» on the diagnostics page: the 2s pullWDiag poll rebuilds
  // #diag-root (and this button) whenever the feed changes, which strips
  // any listener bound in wireContent — delegate at the document level
  // instead, so the binding survives re-renders.
  document.addEventListener("click", (ev) => {
    if (ev.target && ev.target.closest && ev.target.closest("#btn-diag-logs")) switchTab("logs");
  });
  // TUN preflight actions are delegated: the card is patched in place and
  // rebuilt on every state event, which would strip per-node listeners.
  document.addEventListener("click", async (ev) => {
    const el = ev.target && ev.target.closest ? ev.target.closest("[data-tun-act]") : null;
    if (!el) return;
    const act = el.dataset.tunAct;
    el.disabled = true;
    try {
      if (act === "singbox") {
        await updateCore("sing-box");
      } else if (act === "wintun") {
        await installWintun();
      } else if (act === "admin") {
        await api().RestartAsAdmin();
        return;
      } else if (act === "cap") {
        const cmd = "sudo setcap cap_net_admin=+ep " + ((S.tunStatus && S.tunStatus.singbox_path) || "<путь к sing-box>");
        await api().CopyToClipboard(cmd);
        showToast(t("tun.copied"));
      }
    } catch (e) {
      showToast((e && e.message) || e);
    }
    el.disabled = false;
    await refreshTunPreflight();
  });
  window.runtime.EventsOn("profiles", async () => {
    S.profiles = (await api().ListProfiles()) || [];
    if (S.tab === "profiles") {
      renderContent();
    } else if (S.tab === "vpn") {
      // Auto-ping refreshes profiles in the background: never rebuild the
      // VPN tab for latency-only changes — an innerHTML swap recreates the
      // connect button and restarts its animation. Patch the active card
      // in place instead.
      const id = S.state.profile_id || S.selected;
      const p = S.profiles.find((x) => x.id === id);
      const nameEl = document.querySelector("#card-profile .srv-name");
      if (nameEl && p) nameEl.innerHTML = esc(p.name) + tlsBadge(p);
      const pingEl = document.querySelector("#card-profile .srv-ping");
      if (pingEl && p) {
        const lat = p.latency_ms ? (p.latency_ms < 0 ? "n/a" : p.latency_ms + " ms") : "";
        pingEl.textContent = p.host + ":" + p.port + (lat ? " · " + lat : "");
      }
      renderSidebarFooter();
    } else {
      renderSidebarFooter();
    }
  });
  await loadSettings();
  await loadProfiles();
  await loadSubs();
  S.version = await api().Version();
  let st0 = null;
  try { st0 = await api().GetState(); } catch (e) { /* backend not ready */ }
  renderState(st0 || { state: "disconnected" });
  renderExtraFields();
});
