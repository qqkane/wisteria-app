# Wisteria Android

Android-клиент Wisteria. Состоит из двух частей:

1. **Go-движок** — пакет `mobile/` в [протокольном репозитории](https://github.com/qqkane/wisteria)
   (корневой модуль wisteria). gomobile биндит его в `wisteria.aar`:
   дозвон до сервера, Noise-хендшейк (XX/IK по наличию пиннед-ключа),
   один yamux-поток TUN, перекачка сырых IP-пакетов в обе стороны,
   авто-переподключение с backoff и health-монитором.
2. **Android-обёртка** (этот каталог) — Kotlin + `VpnService`. Активити
   принимает ссылки `wisteria://…` (креденшел с ключом), сервис создаёт
   TUN через `Builder.establish()` и передаёт fd движку. Каждый сокет
   движка (TCP-дозвон до сервера, DNS-резолвер) защищается через
   `VpnService.protect()` — иначе трафик зацикливается в туннель.

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

## Установка

Скачайте APK из релиза, разрешите установку из неизвестных источников,
вставьте в приложении ссылку-креденшел от администратора панели
(вкладка «Приглашения» → «Ссылка клиенту») и нажмите «Подключить».

## Ограничения v1

- Один endpoint на профиль: берётся первый сервер из ссылки. Резервные
  серверы из ссылки пока игнорируются (MultiServer не принимает
  пользовательский dialer, а на Android без `protect()` не обойтись).
- Приглашения без ключа (`invite`-ссылки) не поддерживаются — используйте
  ссылку-креденшел, в которой есть ключ клиента.
- APK подписан debug-ключом; обновления «сверху» ставятся только между
  сборками с одинаковым ключом.
