# ShadowTLS v3: внешний TCP-транспорт VetroffProxy

VetroffProxy генерирует совместимые файлы для **официального**
[shadow-tls](https://github.com/ihciah/shadow-tls) (v3 strict). Нужен
отдельный бинарник `shadow-tls` на сервере и клиенте: Vetroff не
заменяет его реализацию криптографии и не запускает этот процесс автоматически.

```text
Vetroff client HTTPS -> local ShadowTLS client :9443
  -> public ShadowTLS server :443 -> local Vetroff HTTPS server :8443
```

ShadowTLS показывает реальное TLS-рукопожатие с внешним сервером,
но его TLS-отпечаток не тождествен Firefox. Внутренний HTTPS
по-прежнему проверяется по сертификату, SNI и SPKI pin.

## Сервер

Нужен домен `vpn.example.org` с действительным TLS-сертификатом.
Сторонний `cover.example.org:443` должен реально поддерживать TLS 1.3.
Это требование строгого режима ShadowTLS v3; `shadowtls-config`
не выполняет сетевых проверок.

```sh
./vetroff setup --server https://vpn.example.org \
  --listen 127.0.0.1:8443 \
  --transport h2-connect --security tls \
  --cert /etc/ssl/vetroff/fullchain.pem \
  --key /etc/ssl/vetroff/privkey.pem --out server-config

./vetroff check-config --config server-config/server.json
```

Создайте отдельный криптографически случайный пароль длиной не менее
16 печатных ASCII-символов в `shadowtls-password` (права 0600).
Сохраните **такой же** пароль на клиенте через защищённый канал.
Это не пароль пользователя Vetroff. Секрет не передаётся в argv.

```sh
./vetroff shadowtls-config --role server \
  --config server-config/server.json --cover cover.example.org:443 \
  --listen 0.0.0.0:443 --password-file shadowtls-password \
  --out server-config/shadowtls-server.json

./vetroff server --config server-config/server.json
# Во втором процессе запускается официальный бинарник:
shadow-tls config --config server-config/shadowtls-server.json
```

Внешний TCP 443 принадлежит ShadowTLS. Сервер Vetroff работает
только на `127.0.0.1:8443` и не должен публиковаться напрямую.
Приватные выходные файлы создаются с правами 0600, без перезаписи.

## Клиент

Передайте обычную приватную ссылку `connection.txt` от Vetroff
и пароль `shadowtls-password` клиенту:

```sh
./vetroff shadowtls-config --role client \
  --link-file connection.txt --cover cover.example.org:443 \
  --listen 127.0.0.1:9443 --password-file shadowtls-password \
  --out shadowtls-client.json --local-link-out connection-local.txt

shadow-tls config --config shadowtls-client.json
# Во втором процессе:
./vetroff check-config --link-file connection-local.txt
./vetroff client --link-file connection-local.txt
```

Для Linux AMD64/ARM64 с проверенным Mozilla runtime можно выбрать:

```sh
./vetroff client --engine firefox --link-file connection-local.txt
```

Локальная ссылка указывает на `127.0.0.1:9443`, но сохраняет SNI
настоящего Vetroff-сервера, его SPKI pin, авторизацию и transport.
Без запущенного ShadowTLS локальное соединение не установится;
прямого fallback к публичному серверу не предусмотрено.

## Совместимость и ограничения

- ShadowTLS v3 — внешний **TCP**-слой. Входящие профили
  `h3-connect` и `h3-post` отвергаются. Режимы H2, WSS и XHTTP
  допускаются, однако их необходимо проверять в паре с сервером.
- Прямые fallback-серверы, Bond и discovery запрещены в этом мосте,
  чтобы трафик не миновал ShadowTLS. Для нескольких нод потребуется
  отдельное доверенное сопоставление каждого ShadowTLS endpoint.
- Прикладные UDP-запросы Vetroff могут идти через UDP-over-HTTP поверх
  TCP, если это поддерживает ваш режим и обе стороны; это не HTTP/3.
- Порт клиента только loopback; серверный внутренний порт тоже
  обязан быть loopback, иначе генератор возвращает ошибку.
- `shadowtls-config` не загружает, не запускает и не обновляет
  оригинальный бинарник. Проверяйте происхождение и контрольную сумму
  [релиза](https://github.com/ihciah/shadow-tls/releases), запускайте
  сервис через systemd или ваш supervisor.
- Не используйте `--security shadowtls-v3` для внутреннего TLS:
  это не заменяет процедуру настройки моста.
- Совпадение с Firefox и успешный обход DPI не гарантируются.
  Для диагностики используйте `vetroff doctor` с собственным echo-сервисом.

[Протокол v3](https://github.com/ihciah/shadow-tls/blob/master/docs/protocol-v3-en.md)
· [Формат JSON](https://github.com/ihciah/shadow-tls/tree/master/examples)
