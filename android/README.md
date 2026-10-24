# Wisteria Android

Android-клиент Wisteria с интерфейсом десктопа. Три части:

1. **Go-движок** — пакет `mobile/` в [протокольном репозитории](https://github.com/qqkane/wisteria)
   (корневой модуль wisteria). gomobile биндит его в `wisteria.aar`:
   дозвон до серверов с ротацией (failover), Noise-хендшейк (XX/IK по
   наличию пиннед-ключа), один yamux-поток TUN, перекачка сырых IP-пакетов
   в обе стороны, авто-переподключение с backoff и health-монитором,
   `Ping` для карточек профилей, `GenerateKeypair` для ручных профилей.
2. **Android-обёртка** (этот каталог, Kotlin). `WisteriaVpnService`
   создаёт TUN через `Builder.establish()` и передаёт fd движку; каждый
   сокет движка защищается через `VpnService.protect()`. Состояние, логи
   и события диагностики идут через `EngineHub` в мост.
3. **UI** — тот же фронтенд, что у десктопа (`assets/ui/` — копия
   `desktop/frontend/dist`: фон с орбами, стеклянные карточки, круглая
   кнопка CONNECT с пульс-кольцами, профили, подписки, темы, RU/EN,
   диагностика). `android-shim.js` реализует над Kotlin-мостом
   (`WisteriaBridge`) те же `window.go.main.App.*` и `window.runtime`,
   что даёт Wails на десктопе; `android.css` складывает сайдбар в
   верхнюю панель и прячет десктоп-специфику (локальные порты, ядра,
   TUN, системный прокси). Настройки/профили/подписки — в
   `AndroidStore` (SharedPreferences, формы данных как в десктопе).

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

## Ограничения v0.2.0

- Протокол wisteria. ss/vmess/vless/trojan/hysteria2/tuic — только в
  десктоп-версии (на Android для них нужен встроенный sing-box).
- Приглашения без ключа (`invite`-ссылки) не поддерживаются — используйте
  ссылку-креденшел, в которой есть ключ клиента.
- Импорт TOML недоступен; обновление APK — вручную из релиза.
- APK подписан debug-ключом; обновления «сверху» ставятся только между
  сборками с одинаковым ключом.
