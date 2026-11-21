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
  qr: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><line x1="14" y1="14" x2="14" y2="21"/><line x1="21" y1="14" x2="21" y2="21"/><line x1="17" y1="14" x2="17" y2="17"/></svg>',
  download: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>',
  upload: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>',
  battery: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="1" y="7" width="17" height="10" rx="2"/><line x1="22" y1="11" x2="22" y2="13"/><line x1="5" y1="11" x2="5" y2="13"/><line x1="9" y1="11" x2="9" y2="13"/></svg>',
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
    "nav.vpn": "VPN", "nav.profiles": "Профили", "nav.logs": "Логи", "nav.diag": "Диагностика", "nav.settings": "Настройки", "ab.home": "Главная", "ab.more": "Ещё",
    "vpn.unprotected": "UNPROTECTED", "vpn.protected": "SECURED", "vpn.connecting": "CONNECTING…",
    "vpn.connect": "Connect", "vpn.disconnect": "Disconnect", "vpn.connected": "Connected",
    "vpn.noProfile": "Профиль не выбран", "vpn.clickSelect": "Нажмите, чтобы выбрать",
    "stat.uptime": "Аптайм", "stat.proxy": "Системный прокси", "stat.core": "Ядро",
    "profiles.title": "Профили", "profiles.empty": "Нет профилей — добавьте ссылку или подписку",
    "profiles.mirrors.one": "+1 зеркало", "profiles.mirrors.few": "+{n} зеркала", "profiles.mirrors.many": "+{n} зеркал",
    "settings.title": "Настройки", "sec.appearance": "Оформление", "sec.connection": "Соединение",
    "sec.general": "Общие", "sec.routing": "Маршрутизация и DNS", "sec.cores": "Ядра", "sec.system": "Система",
    "set.lang": "Язык", "set.socksPort": "Локальный SOCKS-порт", "set.httpPort": "Локальный HTTP-порт",
    "set.sysproxy": "Системный прокси при подключении", "set.bridge": "HTTP-мост (нужен для системного прокси)",
    "set.autoconnect": "Автоподключение при старте", "set.autostart": "Запускать при входе в систему",
    "set.autostartAndroid": "Подключаться при включении телефона",
    "set.wisteriaCore": "Путь к wisteria-client", "set.singboxCore": "Путь к sing-box",
    "set.detect": "Найти ядра", "set.save": "Сохранить настройки",
    "set.startMin": "Запускать свёрнутым", "set.startMinNote": "окно свернётся в панель задач",
    "set.confirmExit": "Спрашивать подтверждение при выходе во время соединения",
    "set.checkIp": "Показывать внешний IP", "set.checkIpNote": "запрос через туннель после подключения",
    "set.autoping": "Автопинг серверов, сек", "set.autopingNote": "0 — выключено",
    "set.listenAll": "Разрешить доступ из локальной сети", "set.listenAllNote": "прокси слушает 0.0.0.0 вместо 127.0.0.1",
    "set.bypassLan": "Обходить локальные адреса",
    "sec.wistRouting": "Маршрутизация wisteria",
    "set.bypassRu": "Не проксировать российские сайты",
    "set.bypassRuNote": "сайты из РФ увидят реальный IP; нужен Android 13+",
    "set.wistDirect": "Свои подсети напрямую",
    "set.wistDirectNote": "CIDR, по одному в строке; домены на Android не поддерживаются",
    "set.wistPresetNote": "по умолчанию для профилей без своего пресета",
    "sec.split": "Split-tunnel по приложениям",
    "set.splitMode": "Режим",
    "set.splitOff": "Весь трафик в туннель",
    "set.splitBypass": "Выбранные — мимо туннеля",
    "set.splitOnly": "Только выбранные — в туннель",
    "set.splitNote": "выбранные приложения видят реальный IP (банки, РФ-сервисы); смена применяется переподключением",
    "set.splitRu": "Российские приложения",
    "set.splitSearch": "Поиск приложения…",
    "set.splitCount": "Выбрано: {n}",
    "set.splitEmpty": "Ничего не найдено",
    "toast.splitRu": "Добавлено RU-приложений: {n}",
    "toast.updPerm": "Разрешите установку из неизвестных источников — обновление продолжится само",
    "set.remoteDns": "Удалённый DNS", "set.remoteDnsNote": "например tcp://8.8.8.8",
    "set.logLevel": "Уровень логирования",
    "set.memLimit": "Лимит памяти ядра, МБ", "set.memLimitNote": "0 — без лимита",
    "set.checkIpNow": "Проверить IP сейчас",
    "toast.ipFail": "Не удалось определить IP",
    "toast.ipRetry": "Туннель переподключается — IP появится через несколько секунд",
    "toast.ipWait": "Определяю IP…",
    "vpn.disconnected": "Disconnected", "vpn.preset": "Пресет обфускации",
    "set.tun": "Режим TUN (весь трафик системы)",
    "set.tunNote": "маршрутизирует всё через туннель; нужен sing-box и wintun.dll; системный прокси при этом не используется",
    "set.tunStack": "Стек TUN",
    "set.tunStrict": "Строгий маршрут (strict route)", "set.tunStrictNote": "надёжнее против утечек; может мешать доступу к локальной сети",
    "set.tabLook": "Внешний вид", "set.tabGeneral": "Основные", "set.tabConn": "Подключение", "set.tabRouting": "Маршрутизация", "set.tabCores": "Ядра",
    "set.routeDirect": "Напрямую, минуя прокси", "set.routeDirectNote": "домены, CIDR, geoip:ru, geosite:ru — по одному в строке",
    "set.routeBlock": "Блокировать", "set.routeBlockNote": "домены, CIDR, geosite:category-ads-all — будут недоступны",
    "set.coreSS": "Ядро для Shadowsocks", "set.coreVmess": "Ядро для VMess", "set.coreVless": "Ядро для VLESS", "set.coreTrojan": "Ядро для Trojan",
    "sec.engines": "Ядро по протоколу",
    "sec.updates": "Обновления",
    "set.updCheck": "Проверять обновления при запуске", "set.updNote": "приложение и ядра sing-box / xray",
    "set.appVersion": "Версия приложения",
    "set.checkUpd": "Проверить обновления", "set.updAvail": "Доступна версия",
    "set.updApply": "Скачать и установить", "set.updRestart": "Перезапустить приложение",
    "set.updCore": "Обновить",
    "toast.coreDl": "Скачивание…",
    "toast.updNone": "актуальная версия",
    "toast.updReady": "Обновление загружено — можно перезапускать",
    "toast.updDl": "Скачиваю APK…", "toast.updApk": "APK скачан — подтвердите установку в системном установщике", "toast.updFail": "не удалось обновить приложение",
    "set.restartAdmin": "Перезапустить от администратора",
    "set.notAdmin": "Без прав администратора TUN недоступен.",
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
    "toast.clipSubs": "подписок: {n}", "toast.clipProfiles": "профилей: {n}", "toast.clipNone": "Ничего нового в буфере", "vpn.tunOn": "TUN активен — весь трафик системы идёт через туннель",
    "toast.stuck": "Подключение зависло — попробуйте ещё раз",
    "traffic.down": "Загрузка", "traffic.up": "Отдача", "traffic.session": "за сессию", "traffic.unsupported": "нет данных",
    "vpn.noProfilesTitle": "Пока нет профилей",
    "vpn.noProfilesText": "Добавьте share-ссылку, подписку или создайте профиль вручную — аккаунт не нужен.",
    "vpn.retry": "Повторить", "vpn.ipHint": "Нажмите, чтобы обновить IP",
    "profiles.search": "Поиск по имени или адресу…",
    "profiles.sortDefault": "Как добавлены", "profiles.sortName": "По имени", "profiles.sortPing": "По пингу",
    "profiles.pingAll": "Пинговать все", "profiles.fastest": "Самый быстрый",
    "profiles.share": "Поделиться", "profiles.edit": "Изменить",
    "profiles.noRaw": "У профиля нет исходной ссылки — он добавлен вручную или из подписки",
    "profiles.fastestNone": "Нет профилей с известным пингом — сначала пропингуйте",
    "profiles.fastestDone": "Включён «{name}» · {ms} ms",
    "edit.title": "Изменить профиль", "edit.save": "Сохранить",
    "qr.title": "Поделиться профилем", "qr.hint": "Наведите камеру другого телефона",
    "qr.copy": "Копировать ссылку", "qr.share": "Отправить",
    "logs.share": "Поделиться", "logs.shareFail": "Не удалось поделиться",
    "diag.test": "Проверить туннель", "diag.testOk": "Туннель работает · {ms} ms", "diag.testFail": "Туннель не отвечает: {err}",
    "subs.used": "Использовано", "subs.left": "Осталось", "subs.unlimited": "без лимита", "subs.expires": "до {date}", "subs.expired": "срок истёк",
    "set.backup": "Резервная копия", "set.export": "Экспорт", "set.import": "Импорт",
    "set.backupNote": "профили, подписки и настройки; файл содержит ключи — храните его в надёжном месте",
    "backup.exported": "Копия сохранена", "backup.imported": "Восстановлено: профилей {p}, подписок {s}", "backup.failed": "Не удалось: {err}",
    "set.battery": "Батарея", "set.batteryNote": "без оптимизации Android не будет убивать туннель",
    "set.batteryOk": "оптимизация отключена", "set.batteryBad": "оптимизация включена — туннель могут убить",
    "set.batteryOpen": "Открыть настройки батареи", "set.vpnOpen": "Системные настройки VPN",
    "pull.refreshing": "Обновляю…", "pull.done": "Обновлено",
    "deeplink.added": "Ссылка добавлена в профили", "deeplink.failed": "Ссылка не распознана",
    "toast.copied": "Скопировано",
  },
  en: {
    "brand": "Wisteria",
    "nav.vpn": "VPN", "nav.profiles": "Profiles", "nav.logs": "Logs", "nav.diag": "Diagnostics", "nav.settings": "Settings", "ab.home": "Home", "ab.more": "More",
    "vpn.unprotected": "UNPROTECTED", "vpn.protected": "SECURED", "vpn.connecting": "CONNECTING…",
    "vpn.connect": "Connect", "vpn.disconnect": "Disconnect", "vpn.connected": "Connected",
    "vpn.noProfile": "No profile selected", "vpn.clickSelect": "Click to choose",
    "stat.uptime": "Uptime", "stat.proxy": "System proxy", "stat.core": "Core",
    "profiles.title": "Profiles", "profiles.empty": "No profiles yet — add a link or a subscription",
    "profiles.mirrors.one": "+{n} mirrors", "profiles.mirrors.few": "+{n} mirrors", "profiles.mirrors.many": "+{n} mirrors",
    "settings.title": "Settings", "sec.appearance": "Appearance", "sec.connection": "Connection",
    "sec.general": "General", "sec.routing": "Routing & DNS", "sec.cores": "Cores", "sec.system": "System",
    "set.lang": "Language", "set.socksPort": "Local SOCKS port", "set.httpPort": "Local HTTP port",
    "set.sysproxy": "Set system proxy on connect", "set.bridge": "HTTP bridge (required for the system proxy)",
    "set.autoconnect": "Auto-connect on start", "set.autostart": "Start on login",
    "set.autostartAndroid": "Connect when the phone boots",
    "set.wisteriaCore": "Path to wisteria-client", "set.singboxCore": "Path to sing-box",
    "set.detect": "Detect cores", "set.save": "Save settings",
    "set.startMin": "Start minimized", "set.startMinNote": "the window opens minimized",
    "set.confirmExit": "Ask for confirmation on exit while connected",
    "set.checkIp": "Show external IP", "set.checkIpNote": "queried through the tunnel after connecting",
    "set.autoping": "Auto-ping servers, sec", "set.autopingNote": "0 — disabled",
    "set.listenAll": "Allow LAN access", "set.listenAllNote": "proxy listens on 0.0.0.0 instead of 127.0.0.1",
    "set.bypassLan": "Bypass local addresses",
    "sec.wistRouting": "Wisteria routing",
    "set.bypassRu": "Do not proxy Russian sites",
    "set.bypassRuNote": "Russian sites see your real IP; needs Android 13+",
    "set.wistDirect": "Custom direct routes",
    "set.wistDirectNote": "CIDRs, one per line; domains are not supported on Android",
    "set.wistPresetNote": "default for profiles without their own preset",
    "sec.split": "Per-app split tunnel",
    "set.splitMode": "Mode",
    "set.splitOff": "All traffic through the tunnel",
    "set.splitBypass": "Selected apps bypass the tunnel",
    "set.splitOnly": "Only selected apps use the tunnel",
    "set.splitNote": "selected apps see the real IP (banks, RU services); changes apply via reconnect",
    "set.splitRu": "Russian apps",
    "set.splitSearch": "Search apps…",
    "set.splitCount": "Selected: {n}",
    "set.splitEmpty": "Nothing found",
    "toast.splitRu": "RU apps added: {n}",
    "toast.updPerm": "Allow installs from unknown sources — the update will continue automatically",
    "set.remoteDns": "Remote DNS", "set.remoteDnsNote": "e.g. tcp://8.8.8.8",
    "set.logLevel": "Log level",
    "set.memLimit": "Core memory limit, MB", "set.memLimitNote": "0 — unlimited",
    "set.checkIpNow": "Check IP now",
    "toast.ipFail": "Could not determine the IP",
    "toast.ipRetry": "Tunnel is reconnecting — the IP will appear in a few seconds",
    "toast.ipWait": "Checking IP…",
    "vpn.disconnected": "Disconnected", "vpn.preset": "Obfuscation preset",
    "set.tun": "TUN mode (all system traffic)",
    "set.tunNote": "routes everything through the tunnel; needs sing-box and wintun.dll; the system proxy is not used",
    "set.tunStack": "TUN stack",
    "set.tunStrict": "Strict route", "set.tunStrictNote": "stronger leak protection; may break LAN access",
    "set.tabLook": "Appearance", "set.tabGeneral": "General", "set.tabConn": "Connection", "set.tabRouting": "Routing", "set.tabCores": "Cores",
    "set.routeDirect": "Direct, bypassing the tunnel", "set.routeDirectNote": "domains, CIDRs, geoip:ru, geosite:ru — one per line",
    "set.routeBlock": "Blocked", "set.routeBlockNote": "domains, CIDRs, geosite:category-ads-all — become unreachable",
    "set.coreSS": "Engine for Shadowsocks", "set.coreVmess": "Engine for VMess", "set.coreVless": "Engine for VLESS", "set.coreTrojan": "Engine for Trojan",
    "sec.engines": "Engine per protocol",
    "sec.updates": "Updates",
    "set.updCheck": "Check for updates on launch", "set.updNote": "the app and the sing-box / xray cores",
    "set.appVersion": "App version",
    "set.checkUpd": "Check for updates", "set.updAvail": "New version available",
    "set.updApply": "Download and install", "set.updRestart": "Restart the app",
    "set.updCore": "Update",
    "toast.coreDl": "Downloading…",
    "toast.updNone": "up to date",
    "toast.updReady": "Update staged — restart to apply",
    "toast.updDl": "Downloading APK…", "toast.updApk": "APK downloaded — confirm the install in the system installer", "toast.updFail": "update failed",
    "set.restartAdmin": "Restart as administrator",
    "set.notAdmin": "TUN needs administrator rights.",
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
    "toast.clipSubs": "subscriptions: {n}", "toast.clipProfiles": "profiles: {n}", "toast.clipNone": "Nothing new in the clipboard", "vpn.tunOn": "TUN active — all system traffic goes through the tunnel",
    "toast.stuck": "Connection is stuck — please try again",
    "traffic.down": "Download", "traffic.up": "Upload", "traffic.session": "this session", "traffic.unsupported": "no data",
    "vpn.noProfilesTitle": "No profiles yet",
    "vpn.noProfilesText": "Add a share link, a subscription, or create a profile by hand — no account needed.",
    "vpn.retry": "Retry", "vpn.ipHint": "Tap to refresh the IP",
    "profiles.search": "Search by name or address…",
    "profiles.sortDefault": "As added", "profiles.sortName": "By name", "profiles.sortPing": "By ping",
    "profiles.pingAll": "Ping all", "profiles.fastest": "Fastest",
    "profiles.share": "Share", "profiles.edit": "Edit",
    "profiles.noRaw": "This profile has no original link — it was added by hand or from a subscription",
    "profiles.fastestNone": "No profile has a known ping — ping them first",
    "profiles.fastestDone": "«{name}» is active · {ms} ms",
    "edit.title": "Edit profile", "edit.save": "Save",
    "qr.title": "Share profile", "qr.hint": "Point another phone's camera at the code",
    "qr.copy": "Copy link", "qr.share": "Send",
    "logs.share": "Share", "logs.shareFail": "Could not share",
    "diag.test": "Test tunnel", "diag.testOk": "Tunnel works · {ms} ms", "diag.testFail": "Tunnel is not responding: {err}",
    "subs.used": "Used", "subs.left": "Left", "subs.unlimited": "unlimited", "subs.expires": "until {date}", "subs.expired": "expired",
    "set.backup": "Backup", "set.export": "Export", "set.import": "Import",
    "set.backupNote": "profiles, subscriptions and settings; the file contains keys — keep it somewhere safe",
    "backup.exported": "Backup saved", "backup.imported": "Restored: {p} profiles, {s} subscriptions", "backup.failed": "Failed: {err}",
    "set.battery": "Battery", "set.batteryNote": "without optimization Android may kill the tunnel",
    "set.batteryOk": "optimization disabled", "set.batteryBad": "optimization enabled — the tunnel may be killed",
    "set.batteryOpen": "Open battery settings", "set.vpnOpen": "System VPN settings",
    "pull.refreshing": "Refreshing…", "pull.done": "Refreshed",
    "deeplink.added": "Link added to profiles", "deeplink.failed": "Link not recognized",
    "toast.copied": "Copied",
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
  tab: "vpn", setTab: "look", theme: "lilac", profiles: [], selected: "", lastLogSeq: 0,
  state: { state: "disconnected" }, uptimeTimer: null, extIP: "",
  // Timestamp of the optimistic "connecting" render (markConnecting): lets
  // the 3 s GetState poll ignore a stale "disconnected" while the Android
  // VPN service is still booting the engine.
  connectingAt: 0,
  subs: [], subBusy: {}, busy: false, // busy = a Connect is in flight
  version: "", upd: null, updReady: false, coreUpd: {},
  apps: null, splitMode: "off", splitApps: [], splitFilter: "",
  wdiag: null, // the Diagnostics page data (wisteria profiles only)
  // Profiles toolbar: a live filter, the chosen ordering and a busy flag for
  // the concurrent ping pass (the bridge caps it at 6 workers).
  pfFilter: "", pfSort: "default", pfPingBusy: false,
  traffic: { rx: 0, tx: 0, rxTotal: 0, txTotal: 0, ok: false },
  battery: null, // {ignoring: bool} — refreshed when Settings → System renders
  pendingShare: "", // link shown in the QR modal
};
const $ = (id) => document.getElementById(id);
// Android build (android-shim.js sets __PLATFORM before this file loads).
// Desktop-only controls are skipped at render time instead of being deleted
// from the DOM afterwards by a MutationObserver.
const isAndroid = window.__PLATFORM === "android";
const esc = (s) => String(s == null ? "" : s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const typeIcon = (tp) => ({ wisteria: I.lock }[tp] || I.globe);
// Bundle links carry mirror endpoints in p.servers; the card shows the
// primary host:port, so name the rest — otherwise a 2-server link looks
// like it lost the second server. Russian needs three plural forms
// (1 зеркало / 2 зеркала / 5 зеркал); the en strings are the same for all
// three keys, so the ru plural rule can pick a key unconditionally.
const mirrorsLabel = (p) => {
  const n = Array.isArray(p.servers) ? p.servers.length - 1 : 0;
  if (n < 1) return "";
  const k = (n % 10 === 1 && n % 100 !== 11) ? "one"
    : (n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 12 || n % 100 > 14)) ? "few" : "many";
  return " · " + t("profiles.mirrors." + k).replace("{n}", n);
};

// TLS badge for wisteria profiles on a real-TLS transport (wave 4). The
// tooltip names the SNI override when one is set, otherwise the default
// (the endpoint host).
function tlsBadge(p) {
  if (!p || p.type !== "wisteria" || p.transport !== "tls") return "";
  const tip = p.tls_sni ? "SNI: " + p.tls_sni : t("tls.sniAuto");
  return ` <span class="tls-badge" title="${esc(tip)}">TLS</span>`;
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
  if (S.tab === "vpn") ensureExtIP();
}

function renderSidebarFooter() {
  const st = S.state;
  // Before the first connect the backend status carries no profile_id —
  // fall back to the profile selected on the Profiles tab, so the choice
  // is visible immediately instead of "no profile selected".
  const p = S.profiles.find((x) => x.id === (st.profile_id || S.selected));
  const dot = st.state === "connected" ? "on" : st.state === "connecting" ? "mid" : "";
  const statusText = st.state === "connected" ? t("vpn.connected") : st.state === "connecting" ? t("vpn.connecting") : t("vpn.unprotected");
  $("sidebar-footer").innerHTML = `
    <div class="sf-status"><div class="sf-dot ${dot}"></div> ${esc(statusText)}</div>
    <div class="sf-server">${esc(p ? p.name : (st.profile_name || "—"))}</div>
    <div class="sf-ip">${esc(p ? p.host + ":" + p.port : (st.socks_addr || ""))}</div>`;
}

// Human-readable byte counts for the traffic card and the subscription bar.
function fmtBytes(n) {
  n = Number(n) || 0;
  if (n < 1024) return n + " B";
  const units = lang === "en" ? ["KB", "MB", "GB", "TB"] : ["КБ", "МБ", "ГБ", "ТБ"];
  let v = n / 1024, i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return (v >= 100 ? v.toFixed(0) : v >= 10 ? v.toFixed(1) : v.toFixed(2)) + " " + units[i];
}

function renderVpn() {
  const st = S.state;
  const conn = st.state === "connected";
  const connecting = st.state === "connecting";
  const reconn = !!st.reconnecting; // always false on Android (no shell-side reconnect field), keeps the markup desktop-identical
  const p = S.profiles.find((x) => x.id === (st.profile_id || S.selected)) || null;
  const label = conn ? t("vpn.protected") : connecting ? t("vpn.connecting") : t("vpn.unprotected");
  const btnText = conn ? t("vpn.connected") : connecting ? t("vpn.connecting") : t("vpn.disconnected");
  const lat = p && p.latency_ms ? (p.latency_ms < 0 ? "n/a" : p.latency_ms + " ms") : "";
  const noProfiles = S.profiles.length === 0;
  return `
    <div class="vpn-screen">
      <div class="vpn-status ${conn ? "on" : connecting || reconn ? "mid" : ""}"><div class="label">${esc(label)}</div></div>
      <div id="btn-wrap" style="position:relative; display:flex; justify-content:center;">
        <button class="connect-btn ${conn ? "connected" : ""} ${connecting || reconn ? "connecting" : ""}" id="btn-toggle" ${connecting ? "disabled" : ""}>
          <div class="ring-anim"></div>
          <div class="ring-anim d2"></div>
          <svg class="spin-arc" viewBox="0 0 100 100" aria-hidden="true"><circle cx="50" cy="50" r="46"/></svg>
          <span class="btn-icons">${I.power}<span class="icon-check">${I.checkStroke}</span></span>
          <span class="connect-text">${esc(btnText)}</span>
        </button>
      </div>
      <div class="profile-card" id="card-profile">
        <div class="icon-box">${typeIcon(p ? p.type : "")}</div>
        <div class="srv-info">
          <div class="srv-name">${esc(p ? p.name : t("vpn.noProfile"))}${tlsBadge(p)}</div>
          <div class="srv-ping">${esc(p ? p.host + ":" + p.port + (lat ? " · " + lat : "") + mirrorsLabel(p) : t("vpn.clickSelect"))}</div>
        </div>
        ${I.chev}
      </div>
      <div class="stats-row">
        <div class="stat-card"><div class="stat-label">${I.clock} ${esc(t("stat.uptime"))}</div><div class="stat-value" id="stat-uptime">${uptimeHTML(st.started_at) || "—"}</div></div>
        <div class="stat-card ${conn ? "stat-click" : ""}" id="card-ip" ${conn ? `title="${esc(t("vpn.ipHint"))}"` : ""}><div class="stat-label">${I.globe} IP</div><div class="stat-value" style="font-size:13px; line-height:2.4;" id="stat-ip">${esc(S.extIP || (conn && S.extIPErr ? "✕" : "—"))}</div></div>
        <div class="stat-card"><div class="stat-label">${I.globe} ${esc(t("stat.proxy"))}</div><div class="stat-value">${st.sys_proxy ? "ON" : "OFF"}</div></div>
      </div>
      ${conn ? `
      <div class="traffic-card" id="traffic-card">
        <div class="tr-item"><span class="tr-label">${I.download} ${esc(t("traffic.down"))}</span><b id="tr-down">${S.traffic.ok ? fmtBytes(S.traffic.rx) + "/s" : "—"}</b></div>
        <div class="tr-item"><span class="tr-label">${I.upload} ${esc(t("traffic.up"))}</span><b id="tr-up">${S.traffic.ok ? fmtBytes(S.traffic.tx) + "/s" : "—"}</b></div>
        <div class="tr-total" id="tr-total">${esc(t("traffic.session"))}: ↓ ${fmtBytes(S.traffic.rxTotal)} · ↑ ${fmtBytes(S.traffic.txTotal)}</div>
      </div>` : ""}
      ${p && p.type === "wisteria" ? `
      <div class="vpn-tun-row">
        <div class="vpn-tun-label">${esc(t("vpn.preset"))}</div>
        <select id="vpn-preset" class="preset-select">
          ${["fast", "balance", "stealth"].map((k) => `<option value="${k}" ${((p.fields && p.fields.preset) || (S.cfg && S.cfg.wisteria_preset) || "stealth") === k ? "selected" : ""}>${k}</option>`).join("")}
        </select>
      </div>` : ""}
      ${noProfiles ? `
      <div class="onboard-card">
        <div class="onboard-icon">${I.shield}</div>
        <h3>${esc(t("vpn.noProfilesTitle"))}</h3>
        <p>${esc(t("vpn.noProfilesText"))}</p>
        <div class="onboard-actions">
          <button class="ghost-btn" id="btn-onboard-clip">${I.clipboard}<span>${esc(t("btn.clip"))}</span></button>
          <button class="ghost-btn" id="btn-onboard-add">${I.plus}<span>${esc(t("add.title"))}</span></button>
        </div>
      </div>` : ""}
      ${st.tun ? `<div class="tun-note">${esc(t("vpn.tunOn"))}</div>` : ""}
      ${st.error ? `<div class="error-box actionable"><span>${esc(st.error)}</span>${p ? `<button class="ghost-btn" id="btn-retry">${esc(t("vpn.retry"))}</button>` : ""}</div>` : ""}
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
        <div class="sli-name">${esc(p.name)}${tlsBadge(p)}</div>
        <div class="sli-addr">${esc(p.host)}:${p.port}${mirrorsLabel(p)}</div>
        ${lat}
      </div>
      ${p.raw ? `<button class="sli-act shareb" title="${esc(t("profiles.share"))}" data-id="${p.id}">${I.share}</button>` : ""}
      ${p.type === "wisteria" ? `<button class="sli-act editb" title="${esc(t("profiles.edit"))}" data-id="${p.id}">${I.edit}</button>` : ""}
      <button class="sli-act pingb" title="ping" data-id="${p.id}">${I.ping}</button>
      <button class="sli-act del" title="delete" data-id="${p.id}">${I.trash}</button>
      <span class="sli-check" style="${active ? "" : "display:none"}">${I.check}</span>
    </div>`;
}

// Subscription quota bar: Subscription-Userinfo is uploaded/downloaded/total
// in bytes and expire in unix seconds. Old subs have no "userinfo" key.
function subUserinfoHTML(u) {
  if (!u) return "";
  const used = (Number(u.upload) || 0) + (Number(u.download) || 0);
  const total = Number(u.total) || 0;
  const left = Math.max(0, total - used);
  const pct = total > 0 ? Math.min(100, Math.round((used * 100) / total)) : 0;
  const exp = Number(u.expire) > 0 ? new Date(Number(u.expire) * 1000) : null;
  const expTxt = exp && !isNaN(exp.getTime())
    ? " · " + (exp.getTime() < Date.now() ? t("subs.expired") : t("subs.expires").replace("{date}", exp.toLocaleDateString()))
    : "";
  return `<div class="sub-usage">
      ${total > 0 ? `<div class="sub-bar"><i style="width:${pct}%"></i></div>` : ""}
      <div class="sub-usage-line">${esc(t("subs.used"))} ${fmtBytes(used)}${total > 0 ? " / " + fmtBytes(total) + " · " + esc(t("subs.left")) + " " + fmtBytes(left) : " · " + esc(t("subs.unlimited"))}${esc(expTxt)}</div>
    </div>`;
}

// Profiles toolbar: a substring filter over name and host:port, plus the
// three orderings the screen needs on a phone (the desktop sorted by hand).
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
    // Known pings first (ascending), then unreachable (-1), then never pinged.
    const rank = (p) => {
      const l = Number(p.latency_ms) || 0;
      return l > 0 ? l : l < 0 ? 1e9 : 2e9;
    };
    arr.sort((a, b) => rank(a) - rank(b));
  }
  return arr;
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
  const visible = (list) => sortProfiles(list.filter(matchProfile));
  const groups = S.subs.map((s) => {
    const meta = s.error
      ? `<div class="sub-meta sub-err">${esc(s.error)}</div>`
      : `<div class="sub-meta">${esc(t("subs.updated"))}: ${esc(fmtSubTime(s.updated_at))}</div>` + subUserinfoHTML(s.userinfo);
    const actions = `
      <button class="sli-act sub-refresh ${S.subBusy[s.id] ? "spin" : ""}" title="${esc(t("subs.refresh"))}" data-id="${s.id}">${I.refresh}</button>
      <button class="sli-act del" title="delete" data-id="${s.id}">${I.trash}</button>`;
    return renderGroup("sub:" + s.id, s.name, meta,
      visible(S.profiles.filter((p) => p.sub_id === s.id)), "subs.emptyGroup", actions);
  }).join("");
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
      <div class="pf-actions">
        <button class="ghost-btn" id="btn-ping-all" ${S.pfPingBusy ? "disabled" : ""}>${I.ping}<span>${esc(t("profiles.pingAll"))}</span></button>
        <button class="ghost-btn" id="btn-fastest">${I.zap}<span>${esc(t("profiles.fastest"))}</span></button>
        <select id="pf-sort" class="pf-sort" title="${esc(t("profiles.sort"))}">
          <option value="default" ${S.pfSort === "default" ? "selected" : ""}>${esc(t("profiles.sortDefault"))}</option>
          <option value="name" ${S.pfSort === "name" ? "selected" : ""}>${esc(t("profiles.sortName"))}</option>
          <option value="ping" ${S.pfSort === "ping" ? "selected" : ""}>${esc(t("profiles.sortPing"))}</option>
        </select>
      </div>
    </div>
    ${groups}
    ${S.subs.length
      ? renderGroup("manual", t("profiles.manual"), "", visible(manual), "profiles.empty", "")
      : profileGrid(visible(manual), "profiles.empty")}`;
}

function renderLogs() {
  return `
    <div class="logs-header">
      <h2>${esc(t("logs.title"))}</h2>
      <div class="logs-tools">
        <button class="ghost-btn" id="btn-copy-logs">${esc(t("logs.copy"))}</button>
        <button class="ghost-btn" id="btn-share-logs">${esc(t("logs.share"))}</button>
        <button class="ghost-btn" id="btn-clear-logs">${esc(t("logs.clear"))}</button>
      </div>
    </div>
    <div class="logs-bar">
      <label class="log-follow"><span class="switch"><input type="checkbox" id="log-follow" checked/><span class="slider"></span></span><span>${esc(t("logs.follow"))}</span></label>
      <div class="log-health" id="log-health" title="${esc(t("logs.healthTip"))}"><span class="lh-dot"></span><span id="log-health-text">—</span></div>
    </div>
    <pre class="log-view" id="log-view"></pre>`;
}

// The live health line: the engine's periodic "health: rtt=… loss=… pings=…"
// reports are consumed by EngineHub (they used to flood this screen); the
// freshest slice is painted here instead, polled while Logs is open.
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
  } catch (e) { /* bridge not ready */ }
}
setInterval(() => { if (S.tab === "logs") pullLogHealth(); }, 2000);

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
  return `<div class="logs-header"><h2>${esc(t("diag.title"))}</h2>
    <button class="ghost-btn" id="btn-test-tunnel">${I.activity}<span>${esc(t("diag.test"))}</span></button></div>
    <div id="diag-root">${S.wdiag ? diagInner(S.wdiag) : ""}</div>`;
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
          : `<button class="ghost-btn" id="btn-apply-upd">${esc(t("set.updApply"))}</button>`}
      </div>` : ""}
    </div>
    <div class="settings-section">
      <h3>${esc(t("sec.general"))}</h3>
      <div class="set-grid">
        <label>${esc(t(isAndroid ? "set.autostartAndroid" : "set.autostart"))}</label>
        <label class="switch"><input id="s-autostart" type="checkbox"/><span class="slider"></span></label>
        ${isAndroid ? "" : `
        <label>${esc(t("set.startMin"))}<span class="set-note">${esc(t("set.startMinNote"))}</span></label>
        <label class="switch"><input id="s-startmin" type="checkbox"/><span class="slider"></span></label>
        <label>${esc(t("set.confirmExit"))}</label>
        <label class="switch"><input id="s-confirmexit" type="checkbox"/><span class="slider"></span></label>`}
        <label>${esc(t("set.checkIp"))}<span class="set-note">${esc(t("set.checkIpNote"))}</span></label>
        <label class="switch"><input id="s-checkip" type="checkbox"/><span class="slider"></span></label>
        ${isAndroid ? "" : `
        <label>${esc(t("set.autoping"))}<span class="set-note">${esc(t("set.autopingNote"))}</span></label>
        <input id="s-autoping" type="number" min="0" max="3600"/>`}
        <label>${esc(t("set.autoconnect"))}</label>
        <select id="s-autoconnect"><option value="">—</option></select>
      </div>
      <div style="margin-top:10px;"><button class="ghost-btn" id="btn-checkip">${esc(t("set.checkIpNow"))}</button></div>
    </div>
    ${isAndroid ? `
    <div class="settings-section">
      <h3>${esc(t("sec.system"))}</h3>
      <div class="set-grid">
        <label>${esc(t("set.battery"))}<span class="set-note" id="battery-note">${esc(t("set.batteryNote"))}</span></label>
        <button class="ghost-btn" id="btn-battery-settings">${I.battery}<span>${esc(t("set.batteryOpen"))}</span></button>
        <label>${esc(t("set.vpnOpen"))}</label>
        <button class="ghost-btn" id="btn-vpn-settings">${esc(t("set.vpnOpen"))}</button>
      </div>
      <div class="settings-sub">${esc(t("set.backup"))}<span class="set-note">${esc(t("set.backupNote"))}</span></div>
      <div class="sys-actions">
        <button class="ghost-btn" id="btn-backup-export">${I.download}<span>${esc(t("set.export"))}</span></button>
        <button class="ghost-btn" id="btn-backup-import">${I.upload}<span>${esc(t("set.import"))}</span></button>
      </div>
    </div>` : ""}`;
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
        <label>${esc(t("set.wistDirect"))}<span class="set-note">${esc(t("set.wistDirectNote"))}</span></label>
        <textarea id="s-wistcidrs" rows="3" placeholder="203.0.113.0/24"></textarea>
        <label>${esc(t("vpn.preset"))}<span class="set-note">${esc(t("set.wistPresetNote"))}</span></label>
        <select id="s-wist-preset">
          <option value="fast">fast</option>
          <option value="balance">balance</option>
          <option value="stealth">stealth</option>
        </select>
      </div>
    </div>
    <div class="settings-section">
      <h3>${esc(t("sec.split"))}</h3>
      <div class="set-grid">
        <label>${esc(t("set.splitMode"))}<span class="set-note">${esc(t("set.splitNote"))}</span></label>
        <select id="s-splitmode">
          <option value="off">${esc(t("set.splitOff"))}</option>
          <option value="bypass">${esc(t("set.splitBypass"))}</option>
          <option value="only">${esc(t("set.splitOnly"))}</option>
        </select>
      </div>
      <div style="display:flex; gap:10px; margin:12px 0; align-items:center; flex-wrap:wrap;">
        <button class="ghost-btn" id="btn-split-ru">${esc(t("set.splitRu"))}</button>
        <span class="core-note" id="split-count" style="margin:0;"></span>
      </div>
      <input id="split-search" type="search" placeholder="${esc(t("set.splitSearch"))}" style="width:100%; margin-bottom:10px;"/>
      <div id="split-list" class="split-list"></div>
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
  S.connectingAt = Date.now();
  renderState({ ...S.state, state: "connecting", profile_id: p.id, profile_name: p.name, error: "" });
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
  try {
    const info = await api().DetectCores();
    const note = $("core-note");
    if (note && info) note.innerHTML = coreNoteHTML(info);
    if (info) relabelCoreButtons(info);
  } catch (e) { /* detection is cosmetic */ }
}

function coreNoteHTML(info) {
  const fmt = (f, p) => (f ? "✓ " + p : "✕ not found");
  return `wisteria-client: ${esc(fmt(info.wisteria_found, info.wisteria_path))}<br>` +
    `sing-box: ${esc(fmt(info.singbox_found, info.singbox_path))}<br>` +
    `xray: ${esc(fmt(info.xray_found, info.xray_path))}`;
}

function relabelCoreButtons(info) {
  const b1 = $("btn-upd-singbox"), b2 = $("btn-upd-xray"), b3 = $("btn-upd-wisteria");
  if (b1) b1.textContent = (info.singbox_found ? t("set.updCore") : t("set.dlCore")) + " sing-box";
  if (b2) b2.textContent = (info.xray_found ? t("set.updCore") : t("set.dlCore")) + " xray";
  if (b3) b3.textContent = (info.wisteria_found ? t("set.updCore") : t("set.dlCore")) + " wisteria";
}

// ---------- per-render event wiring ----------
// Shared by «Из буфера» and the empty-state onboarding card: the clipboard
// may hold subscription URLs, share links or both — the backend routes lines.
async function importFromClipboard() {
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
}

// The IP stat card and «Проверить IP сейчас» share this. GetExternalIP is
// asynchronous on the bridge: an empty answer means "the result will arrive
// through the extip event".
async function checkExtIP() {
  if (S.state.state !== "connected") { showToast(t("vpn.noProfile")); return; }
  try {
    const ip = await api().GetExternalIP();
    if (!ip) { showToast(t("toast.ipWait")); return; }
    S.extIP = ip;
    const el = $("stat-ip");
    if (el) el.textContent = ip;
    showToast("IP: " + ip);
  } catch (e) {
    const msg = (e && e.message) || String(e);
    showToast(msg.indexOf("туннель переподключается") >= 0
      ? t("toast.ipRetry")
      : "✕ " + t("toast.ipFail") + ": " + msg);
  }
}

function closeShareModal() { const m = $("modal-share"); if (m) m.classList.remove("show"); }
function closeEditModal() { const m = $("modal-edit"); if (m) m.classList.remove("show"); }

// Offline QR of the profile's original share link. The encoder lives in
// qr.js (no network, no library): a 61x61 symbol is a few KB of SVG.
function openQrModal(id) {
  const p = S.profiles.find((x) => x.id === id);
  if (!p) return;
  if (!p.raw) { showToast(t("profiles.noRaw")); return; }
  try {
    const r = window.WisteriaQR.qrSvg(p.raw, { quiet: 4 });
    $("qr-wrap").innerHTML = r.svg;
    S.pendingShare = p.raw;
    $("qr-wrap").dataset.name = p.name || "";
    $("modal-share").classList.add("show");
  } catch (e) {
    showToast((e && e.message) || String(e));
  }
}

let editId = "";
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

// Fastest = the lowest positive ping among the currently visible profiles.
// Anything else would silently pick a server the user filtered out.
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

async function testTunnel() {
  try {
    const r = await api().TestTunnel();
    if (r && r.ok) showToast(t("diag.testOk").replace("{ms}", String(r.rtt_ms)));
    else showToast("✕ " + t("diag.testFail").replace("{err}", (r && r.error) || "?"));
  } catch (e) {
    showToast("✕ " + t("diag.testFail").replace("{err}", (e && e.message) || String(e)));
  }
}

async function refreshBatteryNote() {
  const el = $("battery-note");
  if (!el) return;
  try {
    const b = await api().GetBatteryStatus();
    S.battery = b;
    el.textContent = b && b.ignoring ? t("set.batteryOk") : t("set.batteryBad");
    el.classList.toggle("ok", !!(b && b.ignoring));
  } catch (e) { /* older bridge */ }
}

// Segmented control instead of Android's native <select> dialog for the few
// option sets that fit on one row. The select stays in the DOM (hidden) as
// the source of truth, so saveSettings() keeps reading it unchanged.
function segmentify(id) {
  const sel = $(id);
  if (!sel || sel.dataset.segmented === "1") return;
  sel.dataset.segmented = "1";
  const wrap = document.createElement("div");
  wrap.className = "ab-seg";
  Array.from(sel.options).forEach((o) => {
    const b = document.createElement("button");
    b.type = "button";
    b.textContent = o.textContent;
    b.dataset.value = o.value;
    b.classList.toggle("active", o.value === sel.value);
    b.addEventListener("click", () => {
      sel.value = o.value;
      wrap.querySelectorAll("button").forEach((x) => x.classList.toggle("active", x === b));
      sel.dispatchEvent(new Event("change", { bubbles: true }));
    });
    wrap.appendChild(b);
  });
  sel.classList.add("seg-hidden");
  sel.after(wrap);
  sel.addEventListener("change", () => syncSegments());
}

function syncSegments() {
  document.querySelectorAll(".ab-seg").forEach((wrap) => {
    const sel = wrap.previousElementSibling;
    if (!sel || sel.tagName !== "SELECT") return;
    wrap.querySelectorAll("button").forEach((b) => b.classList.toggle("active", b.dataset.value === sel.value));
  });
}

function wireContent() {
  const on = (id, ev, fn) => { const el = $(id); if (el) el.addEventListener(ev, fn); };
  if (isAndroid) { segmentify("s-loglevel"); segmentify("s-tunstack"); segmentify("s-splitmode"); segmentify("s-wist-preset"); }

  on("btn-toggle", "click", async () => {
    if (S.busy) { showToast(t("toast.busy")); return; }
    try {
      if (S.state.state === "connected") {
        S.extIP = ""; S.extIPErr = ""; // new session, new exit IP
        buzz(12);
        api().Disconnect();
      } else {
        if (!S.selected) { openModal(); return; }
        S.extIP = ""; S.extIPErr = ""; // a previous session's IP must not stick
        buzz(10);
        S.busy = true;
        markConnecting(S.selected);
        await withTimeout(api().Connect(S.selected), 45000, t("toast.stuck"));
      }
    } catch (e) {
      showToast((e && e.message) || e);
      // markConnecting drew a local "connecting" screen; a fast backend
      // failure returns with no state event of its own — re-pull the real
      // state so the UI never stays wedged until the 3s reconcile.
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
  document.querySelectorAll(".sli-act.shareb").forEach((b) => b.addEventListener("click", (ev) => {
    ev.stopPropagation();
    openQrModal(b.dataset.id);
  }));
  document.querySelectorAll(".sli-act.editb").forEach((b) => b.addEventListener("click", (ev) => {
    ev.stopPropagation();
    openEditModal(b.dataset.id);
  }));

  // ---- profiles toolbar ----
  on("pf-search", "input", (ev) => {
    S.pfFilter = ev.target.value;
    // Re-render on every keystroke would steal the caret: filter in place by
    // re-rendering the groups only, then restore focus/selection.
    const pos = ev.target.selectionStart;
    renderContent();
    const again = $("pf-search");
    if (again) { again.focus(); try { again.setSelectionRange(pos, pos); } catch (e) {} }
  });
  on("pf-sort", "change", (ev) => { S.pfSort = ev.target.value; renderContent(); });
  on("btn-ping-all", "click", runPingAll);
  on("btn-fastest", "click", pickFastest);

  // subscriptions are added from the clipboard ("From clipboard");
  // AddSubscription remains as an API for future import paths.
  // One-button import: the clipboard may hold subscription URLs, share
  // links or both — the backend routes every line.
  on("btn-clip", "click", importFromClipboard);
  on("btn-onboard-clip", "click", importFromClipboard);
  on("btn-onboard-add", "click", openModal);
  on("btn-retry", "click", () => { const b = $("btn-toggle"); if (b) b.click(); });
  on("card-ip", "click", () => { if (S.state.state === "connected") checkExtIP(); });
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
    document.querySelectorAll(".theme-option").forEach((o) => o.classList.toggle("selected", o === opt));
    await saveSettings();
  }));
  on("btn-checkip", "click", checkExtIP);
  on("btn-share-logs", "click", async () => {
    try { await api().ShareLogs(); }
    catch (e) { showToast(t("logs.shareFail") + ": " + ((e && e.message) || e)); }
  });
  on("btn-test-tunnel", "click", testTunnel);
  on("btn-battery-settings", "click", async () => {
    try { await api().OpenBatterySettings(); setTimeout(refreshBatteryNote, 1200); }
    catch (e) { showToast((e && e.message) || e); }
  });
  on("btn-vpn-settings", "click", async () => {
    try { await api().OpenVpnSettings(); }
    catch (e) { showToast((e && e.message) || e); }
  });
  on("btn-backup-export", "click", async () => {
    try { await api().ExportBackup(); }
    catch (e) { showToast((e && e.message) || e); }
  });
  on("btn-backup-import", "click", async () => {
    try { await api().ImportBackup(); }
    catch (e) { showToast((e && e.message) || e); }
  });
  if ($("battery-note")) refreshBatteryNote();
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
    try {
      const target = (S.upd && (S.upd.url || S.upd.latest)) || "";
      await api().ApplyAppUpdate(target);
    } catch (e) {
      showToast("✕ " + ((e && e.message) || e));
    }
  });
  on("btn-restart-app", "click", async () => {
    try { await api().RestartApp(); }
    catch (e) { showToast((e && e.message) || e); }
  });
  on("btn-upd-singbox", "click", () => updateCore("sing-box"));
  on("btn-upd-xray", "click", () => updateCore("xray"));
  document.querySelectorAll(".set-tab").forEach((b) => b.addEventListener("click", () => {
    S.setTab = b.dataset.settab;
    renderContent();
  }));
  on("btn-save-settings", "click", saveSettings);
  on("s-splitmode", "change", (ev) => { S.splitMode = ev.target.value; });
  on("split-search", "input", (ev) => {
    S.splitFilter = ev.target.value.trim().toLowerCase();
    renderSplitList();
  });
  on("btn-split-ru", "click", async () => {
    try {
      const ru = (await api().ListRuApps()) || [];
      const have = new Set(S.splitApps);
      let n = 0;
      ru.forEach((a) => { if (!have.has(a.pkg)) { S.splitApps.push(a.pkg); n++; } });
      if (S.splitMode === "off") {
        S.splitMode = "bypass";
        const sm = $("s-splitmode"); if (sm) sm.value = "bypass";
      }
      renderSplitList();
      showToast(t("toast.splitRu").replace("{n}", String(n)));
    } catch (e) { showToast((e && e.message) || e); }
  });
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
  S.splitMode = s.split_mode || "off";
  S.splitApps = Array.isArray(s.split_apps) ? s.split_apps.slice() : [];
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
  setc("s-checkip", s.check_ext_ip);
  setv("s-autoping", s.auto_ping_sec || 0);
  setc("s-listenall", s.listen_all);
  setc("s-bypasslan", s.bypass_lan);
  setc("s-bypassru", !s.no_bypass_ru); // checkbox states the feature, the flag states the opt-out
  setv("s-wistcidrs", s.wisteria_direct_cidrs || "");
  setv("s-wist-preset", s.wisteria_preset || "stealth");
  setv("s-splitmode", S.splitMode);
  renderSplitList();
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
  // The form is filled after the segments were built (async GetSettings):
  // move the highlight to the real values, not the template defaults.
  syncSegments();
}

async function saveSettings(opts) {
  const silent = !!(opts && opts.silent);
  const cur = await api().GetSettings();
  const g = (id) => { const el = $(id); return el ? el : null; };
  const num = (id, def) => { const v = parseInt($(id).value, 10); return Number.isFinite(v) ? v : def; };
  const next = {
    language: lang,
    theme: S.theme,
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
    auto_ping_sec: g("s-autoping") ? Math.max(0, num("s-autoping", 0)) : cur.auto_ping_sec,
    listen_all: g("s-listenall") ? $("s-listenall").checked : cur.listen_all,
    bypass_lan: g("s-bypasslan") ? $("s-bypasslan").checked : cur.bypass_lan,
    no_bypass_ru: g("s-bypassru") ? !$("s-bypassru").checked : cur.no_bypass_ru,
    wisteria_direct_cidrs: g("s-wistcidrs") ? $("s-wistcidrs").value.trim() : cur.wisteria_direct_cidrs,
    // Obfuscation preset is edited on the VPN screen (per-profile) and in
    // Settings → Routing (global fallback). Omitting it from this payload
    // reset the global choice to the default on every save.
    wisteria_preset: g("s-wist-preset") ? $("s-wist-preset").value : (cur.wisteria_preset || "stealth"),
    split_mode: S.splitMode,
    split_apps: S.splitApps,
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
  if (!silent) showToast(t("toast.saved"));
}

// Settings save themselves 600 ms after the last edit (the explicit button
// is still there). One save at a time: a second edit during a save queues
// exactly one follow-up instead of racing the first.
let autoSaveTimer = null, autoSaveBusy = false, autoSaveQueued = false;
function scheduleAutoSave() {
  if (S.tab !== "settings") return;
  clearTimeout(autoSaveTimer);
  autoSaveTimer = setTimeout(async () => {
    if (autoSaveBusy) { autoSaveQueued = true; return; }
    autoSaveBusy = true;
    try { await saveSettings({ silent: true }); }
    catch (e) { /* the visible Save button reports failures */ }
    finally {
      autoSaveBusy = false;
      if (autoSaveQueued) { autoSaveQueued = false; scheduleAutoSave(); }
    }
  }, 600);
}

// Any settings control (or a split-tunnel checkbox) is enough of a signal.
// The language select has its own change handler that re-renders first.
function isSettingsControl(el) {
  if (!el) return false;
  const splitBox = !!(el.matches && el.matches("input[data-pkg]"));
  if (!splitBox && (!el.id || !/^s-/.test(el.id))) return false;
  const wrap = $("content-wrapper");
  return !!wrap && wrap.contains(el);
}
document.addEventListener("input", (ev) => { if (isSettingsControl(ev.target)) scheduleAutoSave(); });
document.addEventListener("change", (ev) => { if (isSettingsControl(ev.target)) scheduleAutoSave(); });

// ---------- split tunnel picker ----------
async function loadApps() {
  S.apps = (await api().ListApps()) || [];
}

function renderSplitList() {
  const box = $("split-list");
  if (!box) return;
  const cnt = $("split-count");
  if (cnt) cnt.textContent = t("set.splitCount").replace("{n}", String(S.splitApps.length));
  if (!S.apps) {
    box.innerHTML = `<div class="core-note">…</div>`;
    loadApps().then(renderSplitList).catch(() => {});
    return;
  }
  const have = new Set(S.splitApps);
  const f = S.splitFilter;
  const rows = S.apps.filter((a) => !f || a.label.toLowerCase().includes(f) || a.pkg.includes(f));
  // Selected first so the current bypass set is visible without scrolling.
  const list = rows.filter((a) => have.has(a.pkg)).concat(rows.filter((a) => !have.has(a.pkg))).slice(0, 300);
  box.innerHTML = list.map((a) => `
    <label class="split-row">
      <input type="checkbox" data-pkg="${esc(a.pkg)}" ${have.has(a.pkg) ? "checked" : ""}/>
      <span class="split-name">${esc(a.label)}<span class="split-pkg">${esc(a.pkg)}</span></span>
    </label>`).join("") || `<div class="core-note">${esc(t("set.splitEmpty"))}</div>`;
  box.querySelectorAll("input[data-pkg]").forEach((cb) => cb.addEventListener("change", () => {
    const pkg = cb.dataset.pkg;
    if (cb.checked) { if (!S.splitApps.includes(pkg)) S.splitApps.push(pkg); }
    else S.splitApps = S.splitApps.filter((p) => p !== pkg);
    const c = $("split-count");
    if (c) c.textContent = t("set.splitCount").replace("{n}", String(S.splitApps.length));
  }));
}

function applyTheme() {
  $("app").className = "theme-" + S.theme;
}

// ---------- profiles ----------
async function loadProfiles() {
  S.profiles = (await api().ListProfiles()) || [];
  // A failed state read (engine not up yet, bridge hiccup) must not abort
  // the boot chain and leave the app on a black screen.
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
    await api().RefreshSubscription(id);
  } catch (e) {
    showToast((e && e.message) || e); // the error is also stored on the sub row
    S.subBusy[id] = false;
  }
  // On success the fetch itself is asynchronous (it used to freeze the UI
  // for seconds): the "profiles" event lands when it finishes and clears
  // the spinner together with the fresh data.
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
    // The engine prefixes its own lines with a timestamp; the view adds
    // one from l.at — strip the embedded one so every line reads once.
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
  if (S.tab !== "vpn" || S.state.state !== "connected") return;
  const el = $("stat-uptime");
  if (!el) return;
  const html = uptimeHTML(S.state.started_at);
  if (html) el.innerHTML = html;
}, 1000);

// State reconciliation: the push channel is not bulletproof — an event can
// get lost on the webview bridge and leave the big button stuck on
// "connecting" while the engine is actually connected (observed on a phone
// in 0.4.5). GetState is a cheap read of the engine's shared status;
// repaint whenever the state differs. Only the state string is compared so
// a failure's error box is not wiped by a matching state.
setInterval(async () => {
  if (S.busy) return; // the optimistic connecting state is in flight
  try {
    const st = await api().GetState();
    // Right after Connect the Android VPN service may not have started the
    // engine yet, so its status is still "disconnected": repainting would
    // drop the optimistic CONNECTING screen within 3 s. Give the service a
    // 5 s grace window after the tap (cleared by connected/error).
    if (st && S.state.state === "connecting" && st.state === "disconnected" && !st.error &&
        (Date.now() - S.connectingAt) < 5000) return;
    if (st && st.state && st.state !== S.state.state) renderState(st);
    // The exit-IP check runs on the bridge's own thread and reports through
    // the "extip" event; if that event gets lost on the webview bridge, the
    // card would stay "—" forever. Retry while connected and empty — the
    // bridge rate-limits the actual probes itself.
    if (st && st.state === "connected" && !S.extIP) ensureExtIP();
  } catch (e) { /* bridge not ready yet */ }
}, 3000);

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
  // A real outcome (or a failure) ends the optimistic-connecting grace
  // window used by the 3 s poll.
  if (st && (st.state === "connected" || st.error)) S.connectingAt = 0;
  if (S.state.state === "connecting") sawConnecting = true;
  if (S.state.error && !(prev && prev.error)) buzz([40, 60, 40]); // a failure surfaced
  if (S.tab !== "vpn") { renderSidebarFooter(); return; }
  // Rebuild only on structural changes (profile switch, error box). Same
  // profile transitions (idle → connecting → connected) patch the DOM in
  // place: an innerHTML swap recreates the button node and restarts its CSS
  // animations from frame zero — the connect button stuttered on every
  // state event while the tunnel was coming up, and a tun/sys_proxy flip
  // rebuilt (and re-animated) it exactly when the reveal was running.
  const structural = !prev ||
    prev.profile_id !== S.state.profile_id ||
    (prev.error || "") !== (S.state.error || "");
  if (structural) {
    const prevConn = !!prev && prev.state === "connected";
    if (connTimer) { clearTimeout(connTimer); connTimer = null; }
    lapStart = 0;
    renderContent();
    if (S.state.state === "connected" && !prevConn) beginConnectedLap();
    else if (S.state.state === "connecting") lapStart = performance.now();
    return;
  }
  patchVpnState();
  renderSidebarFooter();
}

// The connected reveal waits for the CSS arc (arc-spin 1.1s) to finish
// exactly one full revolution: a sub-second connect used to swap
// power→check after a quarter lap, and the structural rebuild at connect
// (tun flips false→true) recreated the button born-connected — no
// crossfade at all. Same choreography as the desktop shell.
const SPIN_MS = 1100;
let lapStart = 0;
let connTimer = null;
let sawConnecting = false; // a connecting state was observed this page session
function beginConnectedLap() {
  const btn = $("btn-toggle");
  if (!btn) return;
  if (!sawConnecting) {
    // Boot reconciliation (the tunnel survives app opens): no fake
    // connecting lap — start from the power glyph, crossfade on the live node.
    // The double rAF lets the removed class commit as a real starting
    // style first; a single forced reflow restored the class in the same
    // frame and the CSS crossfade was skipped entirely.
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
  if (!lapStart) lapStart = performance.now();
  const wait = Math.max(0, SPIN_MS - (performance.now() - lapStart));
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
  // Same cleanup the in-place patch does: the shim's cancel markers must
  // not leak from the connecting look into the connected button.
  delete btn.dataset.abCancel;
  btn.classList.remove("ab-cancel");
  if (txt) delete txt.dataset.abCancel;
  lapStart = 0;
  buzz(20);
  renderSidebarFooter();
  if (window.__wisteriaSyncBtn) window.__wisteriaSyncBtn();
}

function patchVpnState() {
  const st = S.state;
  const conn = st.state === "connected";
  const connecting = st.state === "connecting";
  const labelEl = document.querySelector(".vpn-status .label");
  if (labelEl) labelEl.textContent = conn ? t("vpn.protected") : connecting ? t("vpn.connecting") : t("vpn.unprotected");
  const statusEl = document.querySelector(".vpn-status");
  if (statusEl) {
    statusEl.classList.toggle("on", conn);
    statusEl.classList.toggle("mid", connecting);
  }
  // TUN note patched in place: a tun flip must not run the structural
  // innerHTML rebuild (it recreated the button mid-reveal animation). Done
  // before the button block: the connecting → connected branch returns
  // early into beginConnectedLap().
  const screen = document.querySelector(".vpn-screen");
  if (screen) {
    let tunEl = screen.querySelector(".tun-note");
    if (st.tun) {
      if (!tunEl) {
        tunEl = document.createElement("div");
        tunEl.className = "tun-note";
        tunEl.textContent = t("vpn.tunOn");
        const errEl = screen.querySelector(".error-box");
        if (errEl) screen.insertBefore(tunEl, errEl);
        else screen.appendChild(tunEl);
      }
    } else if (tunEl) {
      tunEl.remove();
    }
  }
  const btn = $("btn-toggle");
  if (btn) {
    const wasConn = btn.classList.contains("connected");
    if (conn && !wasConn) { beginConnectedLap(); return; }
    if (!conn && connTimer) { clearTimeout(connTimer); connTimer = null; }
    const wasConnecting = btn.classList.contains("connecting");
    if (connecting && !wasConnecting) lapStart = performance.now();
    if (!connecting && !conn) lapStart = 0;
    btn.classList.toggle("connected", conn);
    btn.classList.toggle("connecting", connecting);
    btn.disabled = connecting;
    const txt = btn.querySelector(".connect-text");
    if (txt) txt.textContent = conn ? t("vpn.connected") : connecting ? t("vpn.connecting") : t("vpn.disconnected");
    // The shim turns the disabled connecting button into an active
    // «Отмена» and marks it via dataset. A full re-render used to drop
    // those markers with the old node; the in-place patch must clear them
    // itself, or the cancel styling leaks into connected/idle states.
    if (!connecting) {
      delete btn.dataset.abCancel;
      btn.classList.remove("ab-cancel");
      if (txt) delete txt.dataset.abCancel;
    }
  }
  if (window.__wisteriaSyncBtn) window.__wisteriaSyncBtn();
}

// ---------- modal ----------
function openModal() {
  $("modal-add").classList.add("show");
  // The TOML tab (and its status node) is removed on Android by
  // applyAndroidLimits: every optional node here must be null-checked.
  const area = $("import-area");
  if (area) area.value = "";
  const errs = $("import-errors");
  if (errs) errs.innerHTML = "";
  const toml = $("toml-status");
  if (toml) toml.innerHTML = "";
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

// Share / edit modals (static markup in index.html).
document.querySelectorAll("#modal-share, #modal-edit").forEach((ov) => {
  ov.addEventListener("click", (ev) => {
    if (ev.target === ov) { closeShareModal(); closeEditModal(); }
  });
});
$("qr-close").addEventListener("click", closeShareModal);
$("edit-close").addEventListener("click", closeEditModal);
$("btn-edit-save").addEventListener("click", saveEditedProfile);
$("btn-qr-copy").addEventListener("click", async () => {
  try { await api().CopyToClipboard(S.pendingShare); showToast(t("toast.copied")); }
  catch (e) { showToast((e && e.message) || e); }
});
$("btn-qr-share").addEventListener("click", async () => {
  try {
    const name = $("qr-wrap").dataset.name || "Wisteria";
    await api().ShareText(S.pendingShare, name);
  } catch (e) { showToast((e && e.message) || e); }
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
  // Wisteria TLS transport (wave 4): the dedicated block replaces the
  // generic text inputs — a select plus two rows that only make sense on TLS.
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
  document.querySelectorAll("#m-extra input[data-field]").forEach((i) => { if (i.value.trim()) fields[i.dataset.field] = i.value.trim(); });
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
  if (ev.target && ev.target.id === "vpn-preset") {
    const pid = S.state.profile_id || S.selected;
    if (pid) {
      await api().SetProfilePreset(pid, ev.target.value);
      await loadProfiles();
      renderContent();
    }
    return;
  }
  if (ev.target && ev.target.id === "s-lang") {
    lang = ev.target.value;
    renderContent();
    await fillSettingsForm();
  }
});

// ---------- Android-only surface limits ----------
// Done once at boot on the real DOM, not from a global MutationObserver:
// the TOML importer is a desktop feature (the bridge answers ImportTOML with
// an honest error), and the manual form only builds wisteria profiles —
// every other protocol arrives through Links / Clipboard.
function applyAndroidLimits() {
  if (!isAndroid) return;
  const tomlTab = document.querySelector('.modal-tab[data-mtab="toml"]');
  if (tomlTab) tomlTab.remove();
  const tomlBody = document.querySelector('.modal-body[data-mbody="toml"]');
  if (tomlBody) tomlBody.remove();
  const sel = $("m-type");
  if (sel) {
    Array.from(sel.options).forEach((o) => { if (o.value !== "wisteria") o.remove(); });
    sel.value = "wisteria";
    renderExtraFields();
  }
}

// "Auto-connect on start": on Android the setting names a profile id (the
// desktop uses a *-toast path for its own). Fire once per page load, only
// when nothing is running — a tunnel that survived an app restart must not
// be rebuilt just because the screen opened.
let autoConnectDone = false;
async function maybeAutoConnect() {
  if (!isAndroid || autoConnectDone) return;
  autoConnectDone = true;
  try {
    const s = S.cfg || await api().GetSettings();
    const id = (s && s.auto_connect) || "";
    if (!id) return;
    if (S.state.state === "connected" || S.state.state === "connecting") return;
    if (!S.profiles.some((p) => p.id === id)) return;
    S.selected = id;
    S.busy = true;
    markConnecting(id);
    await withTimeout(api().Connect(id), 45000, t("toast.stuck"));
  } catch (e) {
    // The service reports failures through "state"; nothing to add here.
  } finally {
    S.busy = false;
  }
}
// ---------- live traffic ----------
// TrafficStats counters are monotonic per process; the bridge keeps the
// session baseline and computes the rate. Poll only while the VPN screen is
// visible and a session is up: on a phone a 1 s timer is real battery.
let trafficBusy = false;
setInterval(async () => {
  if (S.tab !== "vpn" || S.state.state !== "connected" || trafficBusy) return;
  trafficBusy = true;
  try {
    const r = await api().GetTrafficStats();
    if (r) {
      S.traffic = {
        rx: r.rx_speed || 0, tx: r.tx_speed || 0,
        rxTotal: r.rx_total || 0, txTotal: r.tx_total || 0, ok: !!r.ok,
      };
      const d = $("tr-down"), u = $("tr-up"), tot = $("tr-total");
      if (d) d.textContent = r.ok ? fmtBytes(S.traffic.rx) + "/s" : t("traffic.unsupported");
      if (u) u.textContent = r.ok ? fmtBytes(S.traffic.tx) + "/s" : "—";
      if (tot) tot.textContent = t("traffic.session") + ": ↓ " + fmtBytes(S.traffic.rxTotal) + " · ↑ " + fmtBytes(S.traffic.txTotal);
    }
  } catch (e) { /* bridge not ready */ }
  finally { trafficBusy = false; }
}, 1000);

// ---------- pull to refresh (profiles) ----------
// The WebView cannot use the desktop wheel gestures; pulling down at the top
// of the list is the one refresh gesture every phone user knows. The pull is
// passive (native scrolling stays smooth) and only a short drag triggers it.
let ptrStartY = 0, ptrDy = 0, ptrActive = false, ptrBusy = false;
function ptrIndicator() {
  let el = $("ptr-ind");
  if (!el) {
    el = document.createElement("div");
    el.id = "ptr-ind";
    $("app").appendChild(el);
  }
  return el;
}
document.addEventListener("touchstart", (ev) => {
  if (S.tab !== "profiles" || !ev.touches || !ev.touches.length) return;
  const sc = document.querySelector(".app-content");
  if (sc && sc.scrollTop > 0) return;
  ptrStartY = ev.touches[0].clientY;
  ptrDy = 0;
  ptrActive = true;
}, { passive: true });
document.addEventListener("touchmove", (ev) => {
  if (!ptrActive) return;
  ptrDy = Math.max(0, ev.touches[0].clientY - ptrStartY);
  const el = ptrIndicator();
  el.classList.toggle("show", ptrDy > 12);
  el.style.transform = "translateY(" + Math.min(56, ptrDy * 0.6) + "px)";
  el.textContent = ptrDy > 70 ? "↓ " + t("pull.done") : "...";
}, { passive: true });
document.addEventListener("touchend", async () => {
  if (!ptrActive) return;
  ptrActive = false;
  const el = ptrIndicator();
  el.classList.remove("show");
  el.style.transform = "";
  if (ptrDy <= 70 || ptrBusy) return;
  ptrBusy = true;
  showToast(t("pull.refreshing"));
  try {
    await loadSubs();
    await loadProfiles();
    // Bounded: each refresh runs on the bridge's own thread pool; refreshing
    // dozens of subs at once would just queue them behind each other.
    for (const s of S.subs.slice(0, 8)) {
      try { await api().RefreshSubscription(s.id); } catch (e) { /* row keeps its error */ }
    }
    await loadSubs();
    await loadProfiles();
    showToast(t("pull.done"));
  } catch (e) { showToast((e && e.message) || e); }
  finally { ptrBusy = false; }
}, { passive: true });

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
  // Launch-time update check, mirroring the desktop (s-updcheck setting).
  setTimeout(async () => {
    try {
      const s = await api().GetSettings();
      if (s.no_update_check) return;
      const info = await api().CheckAppUpdate();
      if (info && info.has_update) {
        S.upd = info;
        showToast(t("set.updAvail") + ": v" + info.latest);
        if (S.tab === "settings") renderContent();
      }
    } catch (e) { /* offline or rate-limited — stay silent */ }
  }, 4000);
  // «Открыть логи» on the diagnostics page: the 2s pullWDiag poll rebuilds
  // #diag-root (and this button) whenever the feed changes, which strips
  // any listener bound in wireContent — delegate at the document level
  // instead, so the binding survives re-renders.
  document.addEventListener("click", (ev) => {
    if (ev.target && ev.target.closest && ev.target.closest("#btn-diag-logs")) switchTab("logs");
  });
  window.runtime.EventsOn("extip", (d) => {
    // split_mode "only": the app itself is outside the tunnel, so the probe
    // would show the real ISP address. The bridge refuses and explains.
    if (d && d.mode_only) {
      S.extIP = ""; S.extIPErr = d.error || "";
      const el = $("stat-ip");
      if (el) el.textContent = "—";
      showToast(d.error || t("set.splitNote"));
      return;
    }
    S.extIP = (d && d.ip) || "";
    S.extIPErr = (d && d.error) || "";
    const el = $("stat-ip");
    if (el) el.textContent = S.extIP || ((d && d.error) ? "✕" : "—");
    // The probe fires right after connect, when the core may still be
    // reconnecting: explain it instead of showing a scary ✕.
    if (!S.extIP && d && d.error && String(d.error).indexOf("туннель переподключается") >= 0) {
      showToast(t("toast.ipRetry"));
    }
  });
  // A share link opened from a browser/messenger was imported by
  // MainActivity on its own thread; show the result and jump to the list.
  window.runtime.EventsOn("deeplink", async (d) => {
    if (d && d.ok) {
      showToast(t("deeplink.added"));
      await loadSubs();
      await loadProfiles();
      switchTab("profiles");
    } else {
      showToast(t("deeplink.failed"));
    }
  });
  // SAF backup/restore finished (MainActivity writes the file off-thread).
  window.runtime.EventsOn("backup", async (d) => {
    if (!d) return;
    if (d.ok) {
      showToast(d.action === "import"
        ? t("backup.imported").replace("{p}", String(d.profiles || 0)).replace("{s}", String(d.subs || 0))
        : t("backup.exported"));
      if (d.action === "import") {
        await loadSettings();
        await loadProfiles();
        await loadSubs();
      }
    } else {
      showToast(t("backup.failed").replace("{err}", d.error || "?"));
    }
  });
  // The backend auto-pinger emits this; don't wipe an open settings form.
  // Subscription fetches run on their own thread and report through this
  // event too, so it also clears the per-sub refresh spinners.
  window.runtime.EventsOn("profiles", async () => {
    S.profiles = (await api().ListProfiles()) || [];
    S.subs = (await api().ListSubscriptions()) || S.subs;
    S.subBusy = {};
    if (S.tab === "profiles" || S.tab === "vpn") renderContent();
    else renderSidebarFooter();
  });
  await loadSettings();
  await loadProfiles();
  await loadSubs();
  S.version = await api().Version();
  let st0 = null;
  try { st0 = await api().GetState(); } catch (e) { /* engine not ready */ }
  renderState(st0 || { state: "disconnected" });
  renderExtraFields();
  applyAndroidLimits();
  await maybeAutoConnect();
});
