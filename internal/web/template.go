package web

const pageTemplate = `{{define "watchForm"}}
<form method="post" action="{{if .Edit}}/watches/{{.Form.ID}}{{else}}/watches{{end}}">
  <input type="hidden" name="csrf" value="{{.CSRF}}">
  <div class="grid">
    <div><label>Название</label><input name="name" required value="{{.Form.Name}}" placeholder="Крем в GPC"></div>
    <div><label>Метод</label><select name="kind">
      <option value="html" {{if or (eq .Form.Kind "") (selected .Form.Kind "html")}}selected{{end}}>HTML / CSS-селектор</option>
      <option value="psp" {{if selected .Form.Kind "psp"}}selected{{end}}>PSP по URL</option>
    </select></div>
    <div class="full"><label>URL</label><input type="url" name="url" required value="{{.Form.URL}}"></div>
    <div><label>CSS-селектор (для HTML; GPC можно оставить пустым)</label><input name="selector" value="{{.Form.Selector}}" placeholder='meta[name="product:price:amount"]'></div>
    <div><label>Атрибут; пусто = текст</label><input name="attribute" value="{{.Form.Attribute}}" placeholder="content"></div>
    <div><label>Тип значения</label><select name="value_type">
      <option value="price" {{if or (eq .Form.ValueType "") (selected .Form.ValueType "price")}}selected{{end}}>Цена</option>
      <option value="text" {{if selected .Form.ValueType "text"}}selected{{end}}>Текст</option>
    </select></div>
    <div><label>Валюта (пусто = определить из HTML)</label><input name="currency" value="{{.Form.Currency}}" placeholder="GEL"></div>
    <div><label>Правило</label><select name="rule">
      <option value="any_change" {{if or (eq .Form.Rule "") (selected .Form.Rule "any_change")}}selected{{end}}>Любое изменение</option>
      <option value="decrease" {{if selected .Form.Rule "decrease"}}selected{{end}}>Только снижение</option>
      <option value="below" {{if selected .Form.Rule "below"}}selected{{end}}>Ниже порога</option>
    </select></div>
    <div><label>Порог цены (для правила «ниже»)</label><input name="threshold" value="{{threshold .Form.ThresholdMinor}}" placeholder="100.00"></div>
    <div class="full actions">
      <button formaction="{{if .Edit}}/watches/{{.Form.ID}}/preview{{else}}/preview{{end}}">Предпросмотр</button>
      {{if .Edit}}<button>Обновить</button><a class="button muted" href="/">Отмена</a>{{else}}<button>Сохранить</button>{{end}}
    </div>
  </div>
</form>
{{end}}

{{define "page"}}<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Handy Parser</title><style>
:root{font-family:system-ui,sans-serif;color:#17212b;background:#f4f6f8}*{box-sizing:border-box}body{margin:0}.wrap{max-width:1050px;margin:auto;padding:24px}header{display:flex;justify-content:space-between;align-items:center;margin-bottom:22px}h1{font-size:25px}h2{font-size:19px;margin-top:0}.card{background:white;border:1px solid #dfe5ea;border-radius:12px;padding:18px;margin-bottom:18px;box-shadow:0 2px 9px #1020300b}.grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.full{grid-column:1/-1}label{display:block;font-size:13px;color:#50606f;margin-bottom:4px}input,select{width:100%;padding:10px;border:1px solid #bcc8d1;border-radius:7px;background:white}button,.button{border:0;border-radius:7px;padding:9px 13px;background:#146cda;color:white;cursor:pointer;text-decoration:none;font-size:14px}.muted{background:#607080}.danger{background:#b42318}.msg{padding:11px 14px;background:#e8f2ff;border-radius:8px;margin-bottom:16px}.status{font-size:13px;color:#53616e}.error{color:#b42318}.ok{color:#067647}table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:10px 7px;border-bottom:1px solid #e6eaed;vertical-align:top}code{font-size:12px;overflow-wrap:anywhere}.actions{display:flex;gap:7px;flex-wrap:wrap}.login{max-width:380px;margin:12vh auto}.preview{font-size:22px;font-weight:700}@media(max-width:700px){.grid{grid-template-columns:1fr}.wrap{padding:14px}table,thead,tbody,tr,th,td{display:block}th{display:none}td{border:0;padding:4px 0}tr{border-bottom:1px solid #dfe5ea;padding:12px 0}}
</style></head><body><div class="wrap">
{{if .Login}}
  <div class="card login"><h1>Handy Parser</h1>{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}<form method="post" action="/login"><label>Пароль администратора</label><input type="password" name="password" required autofocus><p><button>Войти</button></p></form></div>
{{else}}
  <header><h1>Handy Parser</h1><form method="post" action="/logout"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="muted">Выйти</button></form></header>
  {{if .Message}}<div class="msg">{{.Message}}</div>{{end}}
  {{if .Preview}}
    <div class="card"><h2>Предпросмотр</h2><div class="preview">{{.Observation.Display}}</div>{{if .Observation.Raw}}<p>Исходное значение: <code>{{.Observation.Raw}}</code></p>{{end}}<p>{{.Message}}</p>{{template "watchForm" .}}<p><a href="/">Отмена</a></p></div>
  {{else if .Edit}}
    <div class="card"><h2>Редактировать наблюдение</h2>{{template "watchForm" .}}</div>
  {{else}}
    <div class="card"><h2>Управление</h2><div class="actions"><form method="post" action="/check"><input type="hidden" name="csrf" value="{{.CSRF}}"><button>Проверить всё</button></form>{{if .TelegramConfigured}}<form method="post" action="/telegram/test"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="muted">Тест Telegram</button></form>{{end}}</div><p class="status">Автоматические проверки: {{.Schedule}}.</p>{{if .LastRun}}<p class="status">Последний запуск: {{timevalue .LastRun.StartedAt}}, статус {{.LastRun.Status}}, проверено {{.LastRun.Checked}}, изменений {{.LastRun.Changed}}, ошибок {{.LastRun.Errors}}.</p>{{end}}{{if .PendingOutbox}}<p class="error">Ожидают отправки в Telegram: {{.PendingOutbox}}</p>{{end}}{{if .FailedOutbox}}<p class="error">Не удалось доставить окончательно: {{.FailedOutbox}}. Проверьте настройки Telegram.</p>{{end}}</div>
    <div class="card"><h2>Наблюдения</h2>{{if .Watches}}<table><thead><tr><th>Название</th><th>Значение</th><th>Состояние</th><th>Действия</th></tr></thead><tbody>{{range .Watches}}<tr><td><strong>{{.Name}}</strong><br><code>{{.Kind}}</code></td><td>{{if .LastDisplay}}{{.LastDisplay}}{{else}}—{{end}}<br><span class="status">успешно: {{timefmt .LastSuccessAt}}</span></td><td>{{if .Enabled}}<span class="ok">активно</span>{{else}}пауза{{end}}{{if .LastError}}<br><span class="error">{{.LastError}}</span>{{end}}</td><td><div class="actions"><a class="button" href="/watches/{{.ID}}/edit">Изменить</a><form method="post" action="/watches/{{.ID}}/toggle"><input type="hidden" name="csrf" value="{{$.CSRF}}"><button class="muted">{{if .Enabled}}Пауза{{else}}Включить{{end}}</button></form><form method="post" action="/watches/{{.ID}}/delete" onsubmit="return confirm('Удалить наблюдение и историю?')"><input type="hidden" name="csrf" value="{{$.CSRF}}"><button class="danger">Удалить</button></form></div></td></tr>{{end}}</tbody></table>{{else}}<p>Наблюдений пока нет.</p>{{end}}</div>
    <div class="card"><h2>Добавить страницу</h2>{{template "watchForm" .}}</div>
    {{if .Changes}}<div class="card"><h2>Последние изменения</h2><table><thead><tr><th>Время</th><th>Наблюдение</th><th>Изменение</th></tr></thead><tbody>{{range .Changes}}<tr><td>{{timevalue .ObservedAt}}</td><td>{{.WatchName}}</td><td>{{.OldDisplay}} → {{.NewDisplay}}</td></tr>{{end}}</tbody></table></div>{{end}}
  {{end}}
{{end}}
</div></body></html>{{end}}`
