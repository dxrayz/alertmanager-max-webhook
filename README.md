# Webhook сервис для отправки Alertmanager алертов в МАХ

## Основные особенности

- Токен авторизации `MAX_BOT_TOKEN` передаётся через окружение.
- Сервис позволяет использовать GO шаблоны, стандартный шаблон уже поставляется в коде под формат сообщения html, но так же можно указать директорию с файлами шаблонов в формате glob, которые потом можно переключать пармаетрами webhook запроса.
- Шаблоны можно перезагружать сигналом HUP, без остановки сервиса.
- Реализован функционал разделения сообщений длиной более 4000 символов.

## Формат webhook запроса

```shell
<адрес сервиса>:<порт>/alert/{chat_id}?message-format={html|markdow}&template-name=<имя шаблона>

# <имя шаблона> - это не имя файла, а аргумент функции define в коде шаблона
```

## Флаги конфигурации

```shell
-api-client-timeout string
      MAX API client timeout (default "5")
-listen-address string
      The address to listen on for HTTP requests. (default ":9096")
-templates-path string
      The templates file path in glob format (default "/templates/*.tmpl")
```
