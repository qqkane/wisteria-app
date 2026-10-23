# Wisteria Desktop — метаданные и позиционирование

Store-ready пакет, собранный по фреймворкам [aso-skills](https://github.com/appeeky/aso-skills)
(metadata-optimization, competitor-analysis, keyword-research, app-icon-optimization, app-launch).
Форматы полей — под лимиты Google Play (title ≤30, short description ≤80); для GitHub Releases
работают те же тексты без ограничений.

## Имя и заголовок (title, ≤30 символов)

Варианты по формуле `[Brand] - [Primary Keyword]`:

1. **Wisteria Desktop — VPN Client** (29) ← рекомендуемый: бренд + главная категория
2. **Wisteria: Anti-DPI VPN** (24) — акцент на нишу (anti-DPI), короче
3. **Wisteria VPN & Proxy** (21) — охват второй категории поиска («proxy»)

Логика: бренд «Wisteria» неизвестен вне проекта → по правилу metadata-optimization
ключевое слово должно стоять рядом с брендом, а не вместо него.

## Short description (≤80, RU)

> Быстрый anti-DPI VPN-клиент: wisteria, VLESS, Trojan, Shadowsocks, Hysteria2, TUIC (75)

**EN:**
> Fast anti-DPI VPN client: wisteria, VLESS, Trojan, Shadowsocks, Hysteria2, TUIC (74)

## Keyword field (≤100, без повторов из title)

`anti-dpi,obfuscation,shadowsocks,vless,reality,trojan,hysteria,tuic,tunnel,sing-box,noize,censorship`

Символов: 98/100. Принципы: без бренд-имени, без категории «vpn» (уже в title), без слова
«free», формы единственные.

## Description (hook — первые 170 символов)

**RU:**
> Один клиент для вашего VPN-протокола. Нативный wisteria с анти-DPI обфускацией
> и все популярные протоколы — VLESS REALITY, Trojan, Shadowsocks, Hysteria2, TUIC — в одном окне.

Дальше по структуре metadata-optimization: возможности (4–6 буллетов) → «как работает»
(3 шага: вставили ссылку → выбрали профиль → подключились) → CTA.

## Конкурентный анализ (competitor-analysis framework)

| | v2rayN/NekoRay | Hiddify | Clash-семейство | **Wisteria Desktop** |
|---|---|---|---|---|
| Протоколы | широкий (ядра) | широкий (sing-box) | свой конфиг-формат | широкий (sing-box) + **нативный wisteria** |
| Anti-DPI обфускация | плагинами | частично | отсутствует | **в ядре протокола** (паддинги, тайминги, mimic TLS, rekey) |
| Системный прокси | есть | есть | есть | есть, HKCU без admin |
| Язык RU | частично | есть | частично | **есть по умолчанию** |
| Свой протокол | нет | нет | нет | **да** |

Позиционирование: единственный клиент, для которого wisteria — родной протокол
(та же команда, что и ядро сервера), плюс привычный мультипротокольный режим поверх sing-box.
Ниша: пользователи wisteria-серверов и все, кому нужен запасной мультипротокольный клиент.

Top-opportunity по фреймворку: «Feature Gap» — ни один конкурент не умеет wisteria;
«Market Gap» — RU-язык по умолчанию при anti-DPI фокусе.

## Иконка (app-icon-optimization)

Силуэт грозди глицинии (отсылка к имени Wisteria) белым на индиго→фиолетовом градиенте.
Проверено на читаемость в 16–32 px: силуэт единый, контраст 8:1, без текста.
На сторов иконка выделяется на фоне преобладающих сине-оранжевых «щитов».

## Чек-лист запуска (app-launch, адаптирован под GitHub Releases)

- [ ] Релиз-сборка: `wisteria-client.exe` + `WisteriaDesktop.exe` в zip (workflow `desktop.yml`)
- [ ] Подпись/хеши: прикладывать SHA256SUMS к релизу
- [ ] Скриншоты: дашборд, импорт ссылок, логи (3–5 шт., тёмная тема)
- [ ] Описание релиза = Short description + changelog
- [ ] Проверка на чистой Windows 10/11 (отсутствие WebView2 → сообщение рантайма Wails)
- [ ] Ссылка на скачивание sing-box в README и в подсказке настроек
- [ ] Сбор обратной связи: issues + шаблон баг-репорта

## Ключевые слова для Discovery (keyword-research)

Высокий приоритет (объём × релевантность): `vless client windows`, `vpn для windows`,
`обход блокировок`, `anti-dpi vpn`, `shadowsocks клиент`, `hysteria2 client`,
`xtls reality клиент`, `tuic клиент`, `sing-box gui`.
