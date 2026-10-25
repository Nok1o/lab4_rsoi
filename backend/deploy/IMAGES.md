# Шаг 4. Образы в Docker Hub

Используем один репозиторий `nok1o/lab4_rsoi`.
В нём хранятся четыре разных образа, различаемые тегами:

| Сервис | Пример имени для версии `v1` |
| --- | --- |
| Library | `docker.io/nok1o/lab4_rsoi:library-v1` |
| Rating | `docker.io/nok1o/lab4_rsoi:rating-v1` |
| Reservation | `docker.io/nok1o/lab4_rsoi:reservation-v1` |
| Gateway | `docker.io/nok1o/lab4_rsoi:gateway-v1` |

Версия `IMAGE_TAG` общая для комплекта образов. Префикс сервиса добавляется
автоматически и при сборке, и при рендеринге Helm. Не публикуйте все четыре
образа под одинаковым полным тегом: следующий push заменит предыдущий.
Для повторных публикаций выбирайте новую версию, предпочтительно hash коммита
с исходниками. Скрипт не вычисляет hash сам, потому что исходники могут быть
не закоммичены и такой тег неверно описывал бы сборку.

## 1. Docker и buildx

Для локальной сборки нужен работающий Docker Engine. В нашей конфигурации
его запускает Colima:

```bash
colima start
docker info
docker buildx version
```

Если `docker buildx` не найден, проверьте установку:

```bash
brew list --versions docker-buildx
```

Если пакета нет, установите `brew install docker-buildx`.
Если пакет установлен, но Docker не видит плагин, для этой установки
Homebrew на Apple Silicon можно зарегистрировать его ссылкой:

```bash
mkdir -p ~/.docker/cli-plugins
ln -s /opt/homebrew/lib/docker/cli-plugins/docker-buildx \
  ~/.docker/cli-plugins/docker-buildx
docker buildx version
```

Здесь нет `-f`: существующая ссылка или файл не перезаписывается.
Если ссылка уже есть, сначала проверьте её цель, не удаляйте файл вслепую.
Альтернатива из инструкции Homebrew — добавить каталог плагинов в
`cliPluginsExtraDirs` существующего Docker config, сохранив прочие настройки.

Ноды нашего Kubernetes-кластера имеют архитектуру `amd64`.
Mac — ARM, поэтому `images.sh` явно задаёт `--platform linux/amd64`.
На ARM сборка команд Dockerfile для amd64 может требовать эмуляцию и идти медленнее.
Если появляется `exec format error`, нужно проверить поддержку этой платформы
выбранным builder, а не удалять флаг платформы и публиковать ARM-образ.

## 2. Доступ к registry

В Docker Home откройте Account settings → Personal access tokens → Generate new token.
Создайте PAT с правом публикации образов (Write), без права удаления, если доступен
такой выбор. Задайте срок действия и сохраните токен в защищённом месте.
Не передавайте токен в чат, не добавляйте его в Git или values.

Выполните:

```bash
docker login --username nok1o
```

На запрос `Password` вставьте PAT. Токен не указывайте аргументом `--password`:
так он может попасть в историю команд. Для GitHub Actions позже используем
отдельный secret и неинтерактивный login через стандартный ввод.

В Docker Hub проверьте visibility репозитория:

- Public — кластер может скачать образы без registry Secret.
- Private — потребуется imagePullSecret в namespace приложения.

Смена visibility на Public делает образы доступными всем; не меняйте её,
если в образах есть данные, которые нельзя публиковать.
В образах нашего приложения не должно быть паролей: они поступают из Kubernetes Secrets.

## 3. Просмотр команд без сборки

Команды ниже выполняются из корня `lab4_rsoi`:

```bash
IMAGE_TAG=v1 bash backend/deploy/images.sh plan
```

Режим `plan` только печатает команды build и push. Не обращается к Docker Engine,
не проверяет login и ничего не публикует. Без аргументов тоже используется `plan`.
По умолчанию выбран `nok1o/lab4_rsoi`, переопределение возможно через
`DOCKERHUB_NAMESPACE` и `DOCKERHUB_REPOSITORY`.
Если меняете репозиторий, задавайте одинаковые значения при build и deploy.

## 4. Сборка, затем отдельная публикация

```bash
export DOCKERHUB_NAMESPACE=nok1o
export IMAGE_TAG=v1

bash backend/deploy/images.sh build
```

Скрипт собирает четыре Dockerfile из `backend/deploy/docker` с контекстом `backend`.
`--load` загружает каждый результат в локальное хранилище Docker.
На этом шаге push не выполняется.

Проверьте локальные образы:

```bash
docker image ls nok1o/lab4_rsoi
docker image inspect nok1o/lab4_rsoi:gateway-v1 \
  --format '{{.Os}}/{{.Architecture}}'
```

Ожидаем четыре тега и `linux/amd64` для каждого образа.
После успешной сборки выполняется явная публикация:

```bash
bash backend/deploy/images.sh push
```

Для одного сервиса добавьте его имя, например:

```bash
bash backend/deploy/images.sh build gateway
bash backend/deploy/images.sh push gateway
```

Build/push требуют явно задать `IMAGE_TAG`. Демонстрационное значение `preview`
не допускается. Push использует существующие локальные образы и сам их не собирает.
Повторный push того же тега может перезаписать его в registry.

## 5. Проверка публикации и деплой

После push проверьте вкладку Tags репозитория Docker Hub либо manifest:

```bash
docker manifest inspect nok1o/lab4_rsoi:gateway-v1
```

Когда опубликованы все четыре образа одной версии и готова infra:

```bash
bash backend/deploy/application.sh deploy
kubectl -n library-system get deployments,pods,svc,ingress
```

Скрипт использует ранее экспортированные `DOCKERHUB_NAMESPACE` и `IMAGE_TAG`.
Подробности и диагностика — в `APPLICATION.md`.
GitHub Actions на этом этапе не изменяются.
