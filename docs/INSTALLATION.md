# Установка и эксплуатация

Handy Parser собирается Docker напрямую из исходников репозитория. На сервере не требуется устанавливать Go или другие зависимости приложения. Все пользовательские настройки хранятся в одном файле `.env`.

## 1. Что потребуется

- Debian 13 с Docker Engine и Compose plugin;
- около 800 МБ свободной RAM на время сборки;
- исходящий HTTPS-доступ к контролируемым сайтам и `api.telegram.org`;
- свободный TCP-порт для панели управления.

Проверка Docker:

```sh
docker --version
docker compose version
sudo docker info
```

Если Docker ещё не установлен, используйте [официальную инструкцию Docker для Debian](https://docs.docker.com/engine/install/debian/). Установка Handy Parser не требует изменения или остановки других Compose-проектов.

## 2. Скачать проект

```sh
sudo install -d -m 0755 /opt/handy-parser
sudo chown "$(id -u):$(id -g)" /opt/handy-parser
git clone https://github.com/eXpressionist/handy-parser.git /opt/handy-parser
cd /opt/handy-parser
cp .env.example .env
sudo install -d -o 10001 -g 10001 -m 0700 data
chmod 0600 .env
```

Если каталог уже содержит установленную версию, повторно выполнять `git clone` не нужно.

## 3. Настроить `.env`

Откройте файл:

```sh
nano .env
```

Сгенерировать пароль панели можно командой `openssl rand -hex 24`, затем вставить результат в `HANDY_ADMIN_PASSWORD`.

Основные параметры:

```dotenv
HANDY_WEB_BIND=0.0.0.0
HANDY_WEB_PORT=8080
HANDY_ADMIN_PASSWORD=replace-with-a-long-random-password

HANDY_TELEGRAM_TOKEN=123456789:replace-with-bot-token
HANDY_TELEGRAM_CHAT_ID=123456789

HANDY_DISPLAY_TIMEZONE=Europe/Moscow
HANDY_CHECK_SCHEDULE=00:00,06:00,12:00,18:00
```

- `HANDY_WEB_BIND=0.0.0.0` публикует панель на всех сетевых интерфейсах VPS.
- `HANDY_WEB_PORT` задаёт внешний порт. Внутри контейнера приложение продолжает использовать порт 8080.
- `HANDY_ADMIN_PASSWORD` — отдельный длинный пароль для входа в панель.
- `HANDY_TELEGRAM_TOKEN` — token, выданный `@BotFather`.
- `HANDY_TELEGRAM_CHAT_ID` — ID личного чата или группы для уведомлений.
- `HANDY_CHECK_SCHEDULE` — время автоматических проверок в часовом поясе `HANDY_DISPLAY_TIMEZONE`.

`.env` содержит секреты и уже исключён из Git. Сохраняйте права `0600`, не публикуйте файл и не вставляйте его содержимое в обращения или логи.

### Как получить Chat ID

1. Создайте бота через `@BotFather`.
2. Отправьте новому боту `/start`.
3. Заполните `HANDY_TELEGRAM_TOKEN` в `.env` и выполните скрипт ниже.

Он не выводит token в терминал:

```sh
python3 - <<'PY'
import json
from pathlib import Path
from urllib.request import urlopen

values = {}
for line in Path('.env').read_text().splitlines():
    line = line.strip()
    if line and not line.startswith('#') and '=' in line:
        key, value = line.split('=', 1)
        values[key] = value.strip().strip('"').strip("'")

token = values.get('HANDY_TELEGRAM_TOKEN', '')
if not token:
    raise SystemExit('Сначала заполните HANDY_TELEGRAM_TOKEN в .env')

with urlopen('https://api.telegram.org/bot' + token + '/getUpdates', timeout=20) as response:
    payload = json.load(response)

seen = set()
for update in payload.get('result', []):
    message = update.get('message', {})
    chat = message.get('chat', {})
    if 'id' in chat and chat['id'] not in seen:
        print('chat_id:', chat['id'], 'type:', chat.get('type'))
        seen.add(chat['id'])
if not seen:
    print('Нет обновлений: отправьте боту /start и повторите.')
PY
```

## 4. Собрать и запустить

```sh
cd /opt/handy-parser
sudo docker compose config --quiet
sudo docker compose up -d --build web
sudo docker compose ps
```

Сборка выполняется с одним процессом компиляции, чтобы снизить пиковую нагрузку на слабый VPS. Миграции SQLite применяются автоматически перед запуском панели. После сборки компилятор не попадает в рабочий образ и не расходует память.

Откройте в браузере:

```text
http://IP_СЕРВЕРА:HANDY_WEB_PORT
```

Например, при IP `203.0.113.10` и порте `8080`: `http://203.0.113.10:8080`.

Проверьте firewall VPS. Рекомендуемый начальный вариант — разрешить порт только со своего внешнего IP:

```sh
sudo ufw allow from ВАШ_ВНЕШНИЙ_IP to any port 8080 proto tcp
```

Замените `8080` значением `HANDY_WEB_PORT`. Прямое соединение использует HTTP, поэтому для постоянного доступа через интернет рекомендуется HTTPS reverse proxy. До настройки HTTPS ограничьте порт своим IP или VPN.

## 5. Первичная проверка

1. Войдите с `HANDY_ADMIN_PASSWORD`.
2. Нажмите «Тест Telegram» и убедитесь, что сообщение пришло.
3. Добавьте страницу и сначала используйте «Предпросмотр».
4. Нажмите «Проверить всё» либо выполните:

```sh
sudo docker compose run --rm -T check
```

Для GPC можно оставить CSS-селектор пустым: встроенный профиль подставит стабильный meta-селектор. Для PSP достаточно выбрать метод PSP и указать URL товара.

Первая успешная проверка сохраняет исходное значение и не отправляет уведомление об изменении.

## 6. Автоматические проверки

Планировщик уже работает внутри контейнера `web`; настраивать cron или systemd не нужно. По умолчанию проверки запускаются в 00:00, 06:00, 12:00 и 18:00 по `Europe/Moscow`.

Чтобы изменить расписание, отредактируйте `.env`, например:

```dotenv
HANDY_DISPLAY_TIMEZONE=Europe/Moscow
HANDY_CHECK_SCHEDULE=01:30,07:30,13:30,19:30
```

Применить изменение:

```sh
sudo docker compose up -d --force-recreate web
```

Расписание отображается в блоке управления веб-панели. При совпадении автоматического и ручного запуска SQLite-блокировка оставляет только одну серию проверок.

Если раньше устанавливался systemd timer из предыдущей версии проекта, удалите его один раз:

```sh
sudo systemctl disable --now handy-parser-check.timer 2>/dev/null || true
sudo rm -f /etc/systemd/system/handy-parser-check.timer \
  /etc/systemd/system/handy-parser-check.service
sudo systemctl daemon-reload
```

## 7. Диагностика

```sh
sudo docker compose ps
sudo docker compose logs --tail=100 web
sudo docker stats --no-stream
```

Контейнер `web` ограничен 96 МиБ и остаётся единственным постоянно работающим компонентом. Планировщик спит между запусками и не создаёт дополнительные контейнеры. Сервис `check` в Compose оставлен только для ручной диагностики из командной строки.

## 8. Обновление

```sh
cd /opt/handy-parser
git pull --ff-only
sudo docker compose up -d --build web
```

Перед обновлением рекомендуется сделать резервную копию.

## 9. Резервная копия и восстановление

SQLite работает в режиме WAL, поэтому для простой согласованной копии на короткое время остановите источники записи:

```sh
cd /opt/handy-parser
sudo docker compose stop web
sudo install -d -m 0700 backups
sudo sh -c 'umask 077; tar -czf "backups/handy-parser-$(date +%Y%m%d-%H%M%S).tar.gz" data .env'
sudo docker compose up -d web
```

Архив содержит базу и `.env`, включая пароль и Telegram token. Храните его в защищённом месте вне VPS.

Для восстановления остановите `web`, распакуйте доверенный архив, выполните `sudo chown -R 10001:10001 data`, затем запустите web-компонент командами из раздела 4.

Не используйте `docker compose down -v` и команды глобальной очистки Docker: на VPS работают другие приложения.
