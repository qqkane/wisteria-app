# Wisteria Apps

Пользовательские приложения проекта **wisteria**. Протокол, сервер и ядра живут в
репозитории [qqkane/wisteria](https://github.com/qqkane/wisteria).

## Wisteria Desktop (`desktop/`)

Кроссплатформенный GUI-клиент (Windows + Linux amd64): нативный протокол wisteria
(anti-DPI: Noise + obfuscation + yamux) и популярные прокси-протоколы (ss/vmess/
vless/trojan/hysteria2/tuic) через ядро sing-box. Подробности —
[desktop/README.md](desktop/README.md).

## Релизы и обновления

Пуш тега `v*` в этом репозитории запускает сборку бандлов и публикует релиз:
Windows zip, Linux tar.gz + AppImage, Android APK (десктопные ассеты — с .sha256).
Встроенный автообновлятор приложения читает `releases/latest` отсюда и выбирает
ассет своей платформы.
