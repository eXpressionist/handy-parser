# Установка и эксплуатация

Dockerfile и Compose уже находятся в репозитории. Текущая основная ветка публикуется как multi-arch образ `ghcr.io/expressionist/handy-parser:edge`, однако установка на Debian 13 ещё не прошла приёмочные проверки. До первого тега этот документ служит инструкцией для тестового окружения. Файлы `deploy/*.example` остаются шаблонами systemd и будущего release-развёртывания.

## 1. Требования

- Debian 13, доступ по SSH и sudo, Docker Engine с Compose plugin.
- Исходящие HTTPS-запросы к сайтам, api.telegram.org и GHCR.
- До 10 наблюдений. Около 800 МБ свободной RAM достаточно для проверки выбранной архитектуры, но фактическое потребление измеряется перед релизом.
- Образ для архитектуры VPS: `uname -m` (`x86_64` → amd64, `aarch64` → arm64).

Проверить уже установленный Docker:

```sh
docker --version
docker compose version
sudo docker info
```

Если Docker отсутствует, установить Engine и Compose plugin по [официальной инструкции Docker для Debian](https://docs.docker.com/engine/install/debian/). Использовать apt-репозиторий Docker. Не переустанавливать и не удалять работающие контейнеры других приложений. Docker Engine и containerd имеют собственный расход RAM, который нужно включить в замеры.

## 2. Получить файлы версии

Для тестирования текущей ветки:

```sh
sudo install -d -m 0755 /opt/handy-parser
sudo chown "$(id -u):$(id -g)" /opt/handy-parser
git clone --depth 1 https://github.com/eXpressionist/handy-parser.git /opt/handy-parser
cd /opt/handy-parser
cp deploy/.env.example .env
```

Указанная папка должна быть пустой перед clone. Корневой `compose.yaml` по умолчанию загружает `edge`, а секция `build` позволяет разработчику собрать тот же образ локально. После `v0.1.0` инструкция будет переключена на неизменяемый версионированный тег/digest и deploy-архив с контрольной суммой.

На VPS используется готовый образ: компилировать Go и modernc SQLite на слабом сервере не требуется.

## 3. Telegram и секреты

1. Создать бота через официальный `@BotFather`, сохранить токен только на VPS.
2. Отправить боту `/start` со своего аккаунта.
3. Получить chat_id своего чата через Bot API `getUpdates` локально; не отправлять токен сторонним ботам/сайтам. Для группового чата использовать его chat_id и проверить доступ бота.
4. В `.env` заполнить `HANDY_TELEGRAM_CHAT_ID`. До первого релиза `HANDY_PARSER_IMAGE` задаёт только локальный тег собранного образа; после релиза он будет указывать на опубликованный тег или digest.

Подготовить каталоги:

```sh
sudo install -d -o 10001 -g 10001 -m 0700 data
sudo install -d -o root -g root -m 0700 secrets
sudo install -o 10001 -g 10001 -m 0400 /dev/null secrets/telegram_bot_token
sudo install -o 10001 -g 10001 -m 0400 /dev/null secrets/admin_password
sudo nano secrets/telegram_bot_token
sudo nano secrets/admin_password
sudo chown 10001:10001 secrets/telegram_bot_token secrets/admin_password
sudo chmod 0400 secrets/telegram_bot_token secrets/admin_password
chmod 0600 .env
```

Первый файл содержит токен, второй — длинный уникальный пароль панели; по одному значению без кавычек. Секреты монтируются как файлы. В обычном Docker Compose это не зашифрованное хранилище: исходные файлы на VPS необходимо защищать правами. Проверка прав именно для UID 10001 обязательна в тесте установки.

Для получения chat_id можно выполнить локально на VPS следующий Python-скрипт без помещения токена в историю команд или аргументы процесса. Он не печатает токен и полный ответ с сообщениями:

```sh
sudo python3 - <<'PY'
import json
from pathlib import Path
from urllib.request import urlopen

token = Path('secrets/telegram_bot_token').read_text().strip()
try:
    with urlopen('https://api.telegram.org/bot' + token + '/getUpdates', timeout=20) as response:
        payload = json.load(response)
    seen = set()
    for update in payload.get('result', []):
        message = update.get('message', {})
        chat = message.get('chat', {})
        if chat.get('id') not in seen and 'id' in chat:
            print('chat_id:', chat['id'], 'type:', chat.get('type'))
            seen.add(chat['id'])
    if not seen:
        print('Нет обновлений: отправьте /start новому боту и повторите.')
except Exception:
    print('Не удалось получить обновления; проверьте сеть, токен и отсутствие webhook.')
    raise SystemExit(1)
PY
```

Для продукта рекомендуется отдельный новый бот, чтобы не конфликтовать с webhook или getUpdates существующих приложений. Приложение отправляет уведомления через sendMessage, приём команд Telegram в первой версии не требуется.

## 4. Запустить панель

```sh
sudo docker compose --env-file .env config --quiet
sudo docker compose --env-file .env pull web check
sudo docker compose --env-file .env run --rm -T check migrate
sudo docker compose --env-file .env up -d web
sudo docker compose ps
```

Ожидается контейнер web со статусом healthy. Открытый порт привязан только к localhost VPS. На своём компьютере создать туннель:

```sh
ssh -N -L 18080:127.0.0.1:8080 USER@VPS
```

Открыть `http://localhost:18080` и войти с паролем из admin_password. Для внешнего HTTPS-домена использовать существующий reverse proxy и соответствующий режим secure cookie; прямое публичное открытие 8080 не является штатной настройкой.

## 5. Добавить наблюдения

GPC: исходный URL из research/README.md, метод HTML, селектор `meta[name="product:price:amount"]`, атрибут `content`, тип «цена», валюта GEL. Нажать «Предпросмотр», сверить товар и сохранить.

PSP: метод PSP, исходная ссылка, предпросмотр названия/SKU и действующей цены, затем сохранение. Поле скидочной цены CSS не требуется.

Правило по умолчанию — любое изменение. Первая успешная проверка создаёт исходное значение и не считается изменением.

Ручной запуск всего списка:

```sh
sudo docker compose --env-file .env run --rm -T check
```

Проверить статусы в панели. Кнопка «Тест Telegram» отправляет отдельное контрольное сообщение; постоянные ошибки доставки и ожидающая очередь отображаются в блоке управления.

## 6. Включить расписание

Выбранное расписание по умолчанию: 00:00, 06:00, 12:00, 18:00 **Europe/Moscow**. Оно предварительное и редактируется в timer. `HANDY_DISPLAY_TIMEZONE` отвечает за отображение времени, а не изменяет systemd-расписание.

```sh
sudo install -m 0644 deploy/handy-parser-check.service.example \
  /etc/systemd/system/handy-parser-check.service
sudo install -m 0644 deploy/handy-parser-check.timer.example \
  /etc/systemd/system/handy-parser-check.timer
sudo systemd-analyze verify /etc/systemd/system/handy-parser-check.service \
  /etc/systemd/system/handy-parser-check.timer
systemd-analyze calendar '*-*-* 00,06,12,18:00:00 Europe/Moscow'
sudo systemctl daemon-reload
sudo systemctl enable --now handy-parser-check.timer
systemctl list-timers handy-parser-check.timer
```

`Persistent=true` запускает одну догоняющую проверку после простоя, если было пропущено хотя бы одно срабатывание. Точное срабатывание допускает погрешность до минуты. Повторные серии не должны выполняться параллельно: общая блокировка приложения обязательна.

Проверка через systemd вручную:

```sh
sudo systemctl start handy-parser-check.service
sudo journalctl -u handy-parser-check.service -n 50 --no-pager
```

Временный контейнер имеет фиксированное имя handy-parser-check. ExecStopPost пытается остановить именно его при завершении/прерывании сервиса, чтобы не оставить работающую проверку после timeout. Работоспособность этого поведения проверяется на Debian перед релизом.

## 7. Наблюдение и обслуживание

```sh
sudo docker compose logs --tail=100 web
sudo docker stats --no-stream
sudo systemctl status handy-parser-check.timer
sudo journalctl -u handy-parser-check.service --since today
```

Web ограничен 96 МиБ, check — 192 МиБ. Это пределы, а не измеренное потребление. После проверки контейнер check исчезает; остаются web и инфраструктура Docker. Результаты разовых проверок также сохраняются в БД, потому что логи удалённого контейнера недоступны через compose logs.

Пауза всех проверок: `sudo systemctl stop handy-parser-check.timer`. Уже запущенная серия продолжится; чтобы прервать и её, остановить service. Индивидуальное наблюдение приостанавливается через панель.

## 8. Резервная копия

Для малого объёма достаточно короткой остановки обоих источников записи. Нельзя копировать только основной SQLite-файл во время работы WAL.

```sh
cd /opt/handy-parser
sudo systemctl stop handy-parser-check.timer
sudo systemctl stop handy-parser-check.service
sudo docker compose stop web
sudo install -d -m 0700 backups
sudo sh -c 'umask 077; tar -czf "backups/data-$(date +%Y%m%d-%H%M%S).tar.gz" data .env'
sudo docker compose up -d web
sudo systemctl start handy-parser-check.timer
```

Перед архивированием убедиться, что ручные контейнеры check также завершены. Пароль и токен резервировать отдельно в защищённом хранилище; они не включены в архив выше. Перенести резервную копию вне VPS, ограничить срок хранения.

Восстановление: остановить timer/service/web, сохранить текущую data отдельно, распаковать доверенный архив в пустую папку установки, восстановить владельца 10001:10001 для data и права секретов. Запустить совместимую с этой БД версию образа, проверить историю, затем включить timer. Проверку восстановления провести до первого релиза.

## 9. Обновление и откат

1. Прочитать release notes и совместимость миграций.
2. Скачать новый образ, остановить timer/service/web, сделать согласованную резервную копию.
3. Указать новый фиксированный тег/digest в `.env`, выполнить `compose run --rm -T check migrate`.
4. Запустить web, дождаться healthy, проверить оба источника, включить timer.
5. При необходимости отката вернуть предыдущий образ **и совместимую резервную копию БД**; старый бинарник может не понимать новую схему.

Не применять `docker compose down -v` или очистку Docker ко всем приложениям VPS. Обновление Handy Parser затрагивает только его проект Compose.

## Источники

- [Docker Engine на Debian](https://docs.docker.com/engine/install/debian/).
- [Параметры сервисов Compose](https://docs.docker.com/reference/compose-file/services/).
- [Таймеры systemd в Debian 13](https://manpages.debian.org/trixie/systemd/systemd.timer.5.en.html).
- [Telegram Bot API](https://core.telegram.org/bots/api).
