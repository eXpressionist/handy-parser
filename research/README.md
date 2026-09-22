# Проверка источников цен

Дата: 2026-09-16. Проверено с локального компьютера, не с VPS.

## GPC

- URL: https://gpc.ge/en/details/sun-care/sun-care-face/la-roche-posay-anth-wetskin-50-200ml-5434?product=70229
- Обычный HTTP GET без cookies и выполнения JavaScript: HTTP 200, 274339 байт тела.
- Цена 137.95 GEL присутствует в исходном ответе: элемент с itemprop="price" и content="137.95", метаданные product:price:amount, JSON-LD Product с productID=70229 и offers.price=137.95.
- В браузере значение совпало: 137.95 GEL.
- Для HTTP-парсера предпочтителен селектор `meta[name="product:price:amount"]`, атрибут `content`. Валюта: `meta[name="product:price:currency"]`, атрибут `content`.
- Альтернатива: `[itemprop="price"]`, атрибут `content`, с проверкой единственности результата. В исходном HTML Next.js часть содержимого передаётся в скрытых контейнерах для последующей вставки; длинный селектор по родительским узлам может различаться до и после JavaScript.

## Pharmadepot

Проверено 2026-09-22.

- URL: https://pharmadepot.ge/en/details/sun-care/sun-care-face/la-roche-posay-anthelios-sp50-sensitive-skin-milk-250ml-1123?product=116736
- Обычный HTTP GET с заголовками Handy Parser: HTTP 200, около 255 КБ тела.
- При скидке страница содержит действующую цену 91.33 GEL и старую зачёркнутую цену 140.5 GEL. Длинный DOM-селектор может начать находить оба элемента.
- Метаданные `meta[name="product:price:amount"]` содержат только действующую цену; значение читается из атрибута `content`. Валюта доступна в `meta[name="product:price:currency"]`.
- Для известных доменов GPC и Pharmadepot неоднозначный пользовательский селектор цены может безопасно перейти на канонические метаданные, только если найден ровно один такой элемент.

## PSP

- Исходный URL сохранён в psp-resolve-variables.json (путь без домена).
- Обычный HTTP GET: HTTP 200, 1275 байт, оболочка с пустым `#app`; цены в HTML нет.
- Публичный скрипт страницы `/assets/index.3ac758e3.js` указывает API `https://app.psp.ge/graphql`, GET-запросы и заголовок `store` для языка.
- Запрос urlResolver по пути страницы вернул PRODUCT, ID 83131.
- Запрос productsByID с ID 83131, `store: ka`, `_lang=ka` без cookies и авторизации вернул SKU `95340_1165`, IN_STOCK, final_price=89.67, regular_price=137.95.
- Тело ответа выбранных полей товара: 446 байт. Это размер нашего сокращённого запроса, не всех запросов браузера.
- В браузере длинный CSS-селектор пользователя вернул `89.67₾`; исходная цена в том же ценовом блоке — `137.95₾`.
- Для мониторинга действующей цены использовать `data.productsByID.price_range.maximum_price.final_price.value`. При снятии скидки поле final_price позволяет продолжать мониторинг без зависимости от класса has-discount. Фактический сценарий снятия скидки на этом товаре ещё не наблюдался.
- ID определять по ссылке при добавлении, сохранять вместе с SKU и названием. При ошибках не перезаписывать последнее успешное значение.

## Воспроизведение запросов PSP (Debian)

Из папки research:

```sh
curl --fail-with-body --max-time 30 -sS -G \
  -H 'store: ka' \
  --data-urlencode 'query@psp-resolve.graphql' \
  --data-urlencode 'variables@psp-resolve-variables.json' \
  --data-urlencode '_lang=ka' \
  'https://app.psp.ge/graphql'

curl --fail-with-body --max-time 30 -sS -G \
  -H 'store: ka' \
  --data-urlencode 'query@psp-price.graphql' \
  --data-urlencode 'variables@psp-price-variables.json' \
  --data-urlencode '_lang=ka' \
  'https://app.psp.ge/graphql'
```

## Архитектурный вывод

Обе предоставленные страницы сейчас доступны для мониторинга без браузера: GPC через HTML, PSP через адаптер публичного API сайта. Это не гарантирует аналогичную доступность с IP VPS или неизменность API в будущем.

Предлагается Go + SQLite, разовый процесс проверки по systemd timer четыре раза в сутки, последовательные запросы и отдельный режим лёгкой веб-панели. В интерфейсе: универсальный HTML/CSS-режим и профиль PSP с настройкой по URL. Расход RAM приложения пока не измерялся; рабочее приложение ещё не реализовано.
