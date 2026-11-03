# Wisteria Desktop

Кроссплатформенный GUI-клиент (Windows + Linux amd64) для протокола **wisteria**
(anti-DPI VPN: Noise + obfuscation + yamux)
с поддержкой популярных прокси-протоколов через ядро [sing-box](https://github.com/SagerNet/sing-box).

Архитектура — «оболочка + ядра» (модель v2rayN): приложение является GUI-супервизором,
сами туннели поднимают проверенные ядра в отдельных процессах:

| Протокол | Чем поднимается |
|---|---|
| `wisteria` (нативный) | `bin/wisteria-client.exe` (репозиторий протокола [qqkane/wisteria](https://github.com/qqkane/wisteria), `cmd/wisteria-client`) |
| Shadowsocks, VMess, VLESS (+REALITY), Trojan, Hysteria2, TUIC | `bin/sing-box.exe` (скачайте release с github.com/SagerNet/sing-box) |

## Возможности

- Импорт share-ссылок: `ss://`, `vmess://`, `vless://`, `trojan://`, `hysteria2://` (`hy2://`), `tuic://`, `wisteria://`
- Импорт полного клиентского конфига wisteria (TOML) — endpoint, ключи и обфускация переносятся, приватный ключ клиента сохраняется как есть
- Ручное добавление профиля по полям; TCP-пинг серверов; список профилей с активным
- Локальный SOCKS5 (ядро) + HTTP-мост (оболочка) — системный прокси Windows включается одним тумблером, без прав администратора
- Логи ядра и оболочки в реальном времени; RU/EN интерфейс; автозапуск с Windows; автоподключение
- Профили и настройки — в `%APPDATA%\wisteria-desktop`, файлы 0600

## Сборка

Требуется Go 1.26+. Node.js **не нужен** — фронтенд статический и уже лежит в `frontend/dist`.

```sh
# ядра (из репозитория протокола github.com/qqkane/wisteria)
go build -trimpath -ldflags "-w -s" -o desktop/build/bin/wisteria-client.exe ./cmd/wisteria-client
go build -trimpath -ldflags "-w -s" -o desktop/build/bin/wisteria-server.exe ./cmd/wisteria-server

# приложение
cd desktop
go build -tags desktop,production -ldflags "-w -s -H windowsgui" -o build/WisteriaDesktop.exe .
```

### Linux (amd64)

Сборка и работа поддерживаются; пакет называет бинари без .exe:

```bash
# ядро
# go build -trimpath -ldflags "-w -s" -o desktop/build/bin/wisteria-client ./cmd/wisteria-client

# приложение (нужны GTK3 и WebKit2GTK 4.1 + компилятор для CGO)
# sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev   # debian/ubuntu
# Arch/Manjaro: sudo pacman -S webkit2gtk-4.1
cd desktop
# тег webkit2_41 линкует оболочку с WebKit2GTK 4.1 (на современных
# дистрибутивах ABI 4.0 выпилен; без тега бинарь не запустится)
go build -tags desktop,production,webkit2_41 -ldflags "-w -s" -o build/WisteriaDesktop .
```

Отличия от Windows: системный прокси ставится через gsettings (GNOME и GTK-окружения;
на других DE тумблер честно скажет, что не поддерживается), автозапуск — через
~/.config/autostart, TUN-режим требует запуска от root (или CAP_NET_ADMIN у sing-box).
CI публикует WisteriaDesktop-linux-amd64.tar.gz с той же структурой (бинарь + bin/wisteria-client).

Готовая структура для распространения:

```
WisteriaDesktop/
├── WisteriaDesktop.exe        # оболочка
└── bin/
    ├── wisteria-client.exe    # ядро wisteria (обязательно)
    └── sing-box.exe           # опционально: ss/vmess/vless/trojan/hy2/tuic
```

Оболочка ищет ядра в `bin/` рядом с собой, затем в PATH; путь можно задать в настройках.

## Формат wisteria://

```
wisteria://BASE64URL(server_pubkey)@host:port?pad=64-1448&obf_mu=6.7&obf_sigma=0.6&jitter_us=1&coalesce_ms=5&mimic=1&filler=80-400&filler_ratio=5#Имя
```

- `server_pubkey` — base64 публичного ключа сервера (base64url без паддинга в ссылке; также принимается std/base64url с паддингом). Ключ можно опустить — тогда первый хендшейк пройдёт по XX, и ядро запиннит ключ сервера само.
- Все параметры обфускации опциональны и должны совпадать с серверными.
- Приватный ключ клиента генерируется на устройстве при импорте и никуда не передаётся; публичную половину добавьте в `authorized_peers` сервера.

## Системный прокси

Windows-прокси (WinINET) говорит по HTTP CONNECT, а ядра — по SOCKS, поэтому оболочка
поднимает локальный HTTP-мост (`127.0.0.1:http_port` → SOCKS ядра) и переключает реестр
HKCU + обновляет WinINET. Hostnames уходят в туннель неразрешёнными — резолвит выходной сервер.

## Тесты

```sh
cd desktop
go test ./...
# e2e с настоящими ядрами (поднимает server+client и гоняет трафик через туннель):
WIST_E2E=1 WIST_CLIENT=build/bin/wisteria-client.exe WIST_SERVER=build/bin/wisteria-server.exe \
  go test ./internal/core/ -run TestServiceE2E -v
```

## Дорожная карта

- Трей-иконка (сворачивание в трой, быстрое подключение)
- TUN-режим (системный VPN, требует прав администратора)
- PAC-режим и раздельная маршрутизация (bypass_ru)
- Автообновление ядер

## Лицензии

Код оболочки — под лицензией репозитория. Ядро sing-box не входит в состав и
распространяется по своей лицензии (GPLv3+) — скачивайте с официального репозитория.
