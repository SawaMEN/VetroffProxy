# Запуск

[Документация](README.md) · [Настройки](FEATURES.md) · [Диагностика](CONFIGURATION.md)

Нужны домен с DNS-записью на сервер, открытый TCP 443 и действительный
TLS-сертификат. Для HTTP/3 также откройте UDP-порт сервера.

Скачайте полный архив своей архитектуры из [Releases](https://github.com/SawaMEN/VetroffProxy/releases).
Linux AMD64/ARM64 архивы сохраняйте целиком: `vetroff`, `lib/`, `runtime/`, helper и метаданные.
Запускайте CLI из распакованного каталога `vetroff-necko/`.
[Проверка происхождения и обновление](OPERATIONS.md).

Обычная Go-сборка без нативного Mozilla runtime (Go 1.26.10+):

```sh
go build -o vetroff ./cmd/vetroff
```

## Сервер

Замените домен и пути на свои; каталог `server-config` ещё не должен существовать.

```sh
./vetroff setup --server https://vpn.example.com \
  --cert /etc/ssl/vetroff/fullchain.pem --key /etc/ssl/vetroff/privkey.pem \
  --out server-config
./vetroff check-config --config server-config/server.json
./vetroff server --config server-config/server.json
```

`server.json` — приватная конфигурация; `connection.txt` — профиль клиента с паролем.
`node.json` и `cluster.json` описывают ноды для Bond/discovery; второй сервер не нужен.
Сохраните созданный каталог целиком.
Храните файлы вне публичного сайта; постоянный сервер запускайте через службу ОС.

## Клиент

Передайте `connection.txt` клиенту по защищённому каналу и ограничьте доступ владельцем.

```sh
./vetroff check-config --link-file connection.txt
./vetroff client --link-file connection.txt
curl --socks5-hostname 127.0.0.1:1080 https://example.com/
```

Укажите в приложении SOCKS5 `127.0.0.1:1080` и DNS через прокси.
Для всего устройства нужен отдельный VPN-клиент с TUN и маршрутизацией.
Параметры транспорта задаются при `setup`; доступные флаги — `./vetroff client --help`.

Явно выбрать стандартную защищённую передачу можно через `setup --security tls`
или `client --security tls` (без профиля). Это тот же TLS 1.3, что уже
используется по умолчанию, а не новый протокол.

### Нативный Firefox-транспорт (Linux AMD64/ARM64)

В полном Mozilla-архиве можно включить строгий режим:

```sh
./vetroff client --engine firefox --link-file connection.txt
```

Он использует настоящий Necko для HTTP/2 или HTTP/3, согласованный с профилем
подключения. Нельзя подменять его Go TLS/quic-go: при отсутствии проверенного
runtime или неподдерживаемой настройке запуск завершится ошибкой. Если `vetroff`
запускается отдельно от своего архива, укажите `--necko-bundle /path/to/bundle`.

Создавать собственный IP-протокол вместо TCP/UDP для такого режима не следует:
обычный Firefox работает через TCP (HTTP/2) или UDP/QUIC (HTTP/3).
У Vetroff остаётся собственный защищённый прикладной туннель внутри HTTPS;
это не делает поток неотличимым от обычного посещения сайтов.
