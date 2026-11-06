# Wisteria Android

Android-клиент Wisteria с интерфейсом десктопа. Три части:

1. **Go-движок** — пакет `mobile/` в [протокольном репозитории](https://github.com/qqkane/wisteria)
   (корневой модуль wisteria). gomobile биндит его в `wisteria.aar`:
   дозвон до серверов с ротацией (failover), Noise-хендшейк (XX/IK по
   наличию пиннед-ключа), один yamux-поток TUN, перекачка сырых IP-пакетов
   в обе стороны, авто-переподключение с backoff и health-монитором,
   `Ping` для карточек профилей, `GenerateKeypair` для ручных профилей.
   Резолв имени сервера: UDP-53 с DoH-фолбэком по TCP/443 — сети, дропающие
   сторонний plaintext-DNS, больше не ломают дозвон (но SERVFAIL самой зоны
   фолбэком не лечится — нужны живые DNS или IP-эндпоинты в профиле).
2. **Android-обёртка** (этот каталог, Kotlin). `WisteriaVpnService`
   создаёт TUN через `Builder.establish()` и передаёт fd движку; каждый
   сокет движка защищается через `VpnService.protect()`. Состояние, логи
   и события диагностики идут через `EngineHub` в мост. Профиль
   wisteria дополнительно несёт TLS-поля транспорта (wave 4):
   `transport` (`"tls"` или отсутствие = обычный TCP, `"tcp"`
   нормализуется в отсутствие при сохранении), `tls_sni` (необязательный
   SNI; по умолчанию — хост endpoint; только домен по RFC 1123, IP-литералы
   запрещены — RFC 6066) и `tls_insecure` (не проверять сертификат,
   только вместе с `transport = "tls"` — лабораторные самоподписанные
   развёртывания). Правила валидации — `TlsTransport.kt`, те же, что у
   десктопа и загрузчика конфига ядра; проверка происходит в одной точке —
   `AndroidStore.addProfile`. Импорт ссылок (`mobile.ParseLinks`)
   переносит ключи `transport`/`tls_sni`/`tls_insecure` из строк
   `servers[]` в профиль; конфиг для `Start()` собирает `servers[]`
   и для ручного профиля (одна строка из host:port + ключи TLS), так что
   TLS доезжает до движка в любом пути. До aar с движком wave 4
   (следующий тег CI) TLS-поля сохраняются, но реального TLS не будет —
   см. «Сборка».
3. **UI** — тот же фронтенд, что у десктопа (`assets/ui/` — копия
   `desktop/frontend/dist`: фон с орбами, стеклянные карточки, круглая
   кнопка CONNECT с пульс-кольцами, профили, подписки, темы, RU/EN,
   диагностика). `android-shim.js` реализует над Kotlin-мостом
   (`WisteriaBridge`) те же `window.go.main.App.*` и `window.runtime`,
   что даёт Wails на десктопе; `android.css` складывает сайдбар в
   верхнюю панель и прячет десктоп-специфику (локальные порты, ядра,
   TUN, системный прокси). Настройки/профили/подписки — в
   `AndroidStore` (SharedPreferences, формы данных как в десктопе).
   Ручная форма для wisteria содержит «Транспорт» (TCP/TLS), «TLS SNI»
   и «Не проверять сертификат» (видны при TLS); карточки профилей с TLS
   помечены бейджем `TLS` с подсказкой SNI.

Netstack (gVisor и т.п.) не нужен: сервер релеит сырые IP-пакеты,
VpnService отдаёт их напрямую в fd.

## Сборка

Нужны: Go (см. wisteria/go.mod), JDK 17, Android SDK, NDK, Gradle 8.9,
gomobile (`go install golang.org/x/mobile/cmd/gomobile@latest` и `gobind`).

```sh
# 1. AAR движка (из чекаута протокольного репо)
export ANDROID_NDK_HOME=<ndk>
gomobile init
gomobile bind -target=android -androidapi 24 -javapkg=org.wisteria \
  -o <path-to-wisteria-app>/android/app/libs/wisteria.aar ./mobile

# 2. APK
gradle -p android assembleDebug   # или assembleRelease (подписан debug-ключом)
```

CI делает всё то же самое автоматически (`.github/workflows/android.yml`);
на тегах `v*` APK прикладывается к релизу как `Wisteria-android.apk`.

## Ограничения

- Приглашения без ключа (`invite`-ссылки) не поддерживаются — используйте
  ссылку-креденшел, в которой есть ключ клиента.
- Импорт TOML недоступен; обновление APK — вручную из релиза.
- AAR собирается только под arm64/arm (CI так режет размер APK): на
  x86_64-эмуляторах приложение падает при старте — Go-рантайм несовместим
  с ARM-трансляцией эмулятора.
- Release-APK подписан постоянным ключом (восстанавливается из CI-секретов),
  поэтому обновления ставятся поверх предыдущих релизов.
