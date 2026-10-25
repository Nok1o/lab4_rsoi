# Шаги 2–3. Go-сервисы и Ingress

Команды запускаются из корня `lab4_rsoi`.
Этот шаг описывает развёртывание, но не собирает и не публикует образы.
Корневой README и Go-код не изменяются.

## Один chart, четыре release

Универсальный chart находится в `helm/service`.
В нём нет условий вида «если это Rating Service»: различия задают values.

| Values-файл | Helm release / Deployment / Service | HTTP-порт | Префикс тега образа | БД |
| --- | --- | --- | --- | --- |
| `helm/values/library.yaml` | `library-service` | 8060 | `library-` | `libraries` |
| `helm/values/rating.yaml` | `rating-service` | 8050 | `rating-` | `ratings` |
| `helm/values/reservation.yaml` | `reservation-service` | 8070 | `reservation-` | `reservations` |
| `helm/values/gateway.yaml` | `gateway-service` | 8080 | `gateway-` | Нет, использует Redis |

Все четыре образа хранятся в одном репозитории `nok1o/lab4_rsoi` и различаются
префиксом тега. Например, версия `v1` даёт теги `library-v1`, `rating-v1`,
`reservation-v1`, `gateway-v1`. Внутри generic chart `image.tagPrefix`
по умолчанию пуст: формат тега для сервисов задаётся только values-файлами.

Chart создаёт Deployment и ClusterIP Service для каждого release.
Только `gateway.yaml` включает обычный Kubernetes Ingress класса `traefik`.
Изменение одного release не требует переустановки остальных.
Все сервисы устанавливаются в `library-system`, рядом с infra.

## Что делает каждый шаблон

- `Chart.yaml` — описание пакета и его версия, не версия Go или образа.
- `values.yaml` — общие настройки: реплики, порт, ресурсы, probes, Ingress.
- `templates/_helpers.tpl` — общие labels и сборка полного имени образа.
- `templates/deployment.yaml` — Pod с Go-процессом, окружением и проверками здоровья.
- `templates/service.yaml` — внутренний адрес, который не зависит от IP Pod.
- `templates/ingress.yaml` — необязательный внешний HTTP-маршрут.

Приложения не хранят данные в своей файловой системе, поэтому им не нужны PVC.
Deployment заменяет экземпляры при обновлении и поддерживает число реплик.
При RollingUpdate допускается один дополнительный Pod и не допускается
умышленное уменьшение числа готовых экземпляров: `maxSurge=1`,
`maxUnavailable=0`. Это не гарантия отсутствия любых сбоев.
Контейнеры работают не от root, с read-only корневой ФС и без Kubernetes API-токена.

HTTP-порт задаётся один раз через `port`. Chart использует его для контейнера,
Service и переменной `PORT`; добавлять второй `PORT` в `env` не нужно.
Параметры `env` имеют стандартный формат Kubernetes и сохраняют порядок.
Requests/limits, число реплик и параметры probes можно переопределять в values.

## Подключение к infra и Secrets

У Library, Reservation и Rating заданы `DB_HOST`, `DB_PORT`, `DB_NAME`,
`DB_USER`, `DB_SSLMODE`. Существующие Go-конфиги собирают из них DATABASE_URL,
включая корректное URL-экранирование пароля.

`DB_PASSWORD` берётся через `secretKeyRef` из своего ключа
`postgresql-credentials`. Ни один из трёх контейнеров не получает пароль
администратора или пароли двух остальных сервисов.

Gateway получает URL трёх внутренних HTTP-сервисов и пароль из
`redis-credentials`. В списке env сначала задан `REDIS_PASSWORD`, затем:

```yaml
- name: REDIS_URL
  value: redis://:$(REDIS_PASSWORD)@library-infra-redis:6379/0
```

Kubernetes подставляет ранее заданную переменную при создании контейнера.
Поэтому env — список, а не map, ключи которого Helm мог бы отсортировать.
Сгенерированный в инструкции infra hex-пароль безопасен для такой URL-подстановки.
Если вручную заменить его паролем с `@`, `:`, `/` и другими специальными
символами, потребуется URL-экранирование или отдельно подготовленный Redis URL:
сама подстановка Kubernetes значения не экранирует.

Адреса соответствуют infra release `library-infra` и обычным внутренним Service.
Если переименовать infra release или удалить обычные Service в пользу headless,
нужно соответственно обновить адреса в values. Этот шаг infra не изменяет.
Соединения с PostgreSQL/Redis внутри кластера пока без TLS; наружу базы не открываются.

## Проверки здоровья

Все четыре сервиса уже имеют `GET /manage/health`. Используем его в:

- startupProbe — даёт процессу время запуститься;
- readinessProbe — определяет готовность Pod принимать трафик;
- livenessProbe — перезапускает неотвечающий контейнер после серии ошибок.

Эта ручка проверяет ответ самого HTTP-процесса, а не доступность зависимостей.
Падение Rating не должно вызывать каскадные перезапуски Gateway:
это ситуация для circuit breaker и логики деградации, не для liveness.
Зависимости всё ещё нужны при первоначальном запуске: Go-сервисы проверяют БД,
а Gateway инициализирует очередь Redis. При неготовой infra контейнер может
завершиться и быть перезапущен Kubernetes. Probes не заменяют интеграционные тесты.

## Как проходит внешний запрос

```text
Клиент → существующий балансировщик → Traefik
       → Ingress gateway-service
       → Service gateway-service:8080
       → Pod Gateway
       → внутренние Service Library / Reservation / Rating
```

Ingress направляет `/` и вложенные пути в Gateway, не изменяя URL.
Поэтому работают как `/api/v1/...`, так и `/manage/health`.
Host не задан: можно обращаться по существующему внешнему IP без настройки DNS.
HTTPS и сертификат на этом шаге не настраиваются.
Новых LoadBalancer Service, NodePort и облачных балансировщиков chart не создаёт.
ClusterIP не является NetworkPolicy: другие Pod кластера всё ещё могут обращаться
к внутренним адресам при наличии сетевой связности.

## Проверка до создания Docker Hub аккаунта

```bash
bash backend/deploy/application.sh lint
bash backend/deploy/application.sh template gateway
```

По умолчанию скрипт использует Docker Hub namespace `nok1o` и версию `preview`
только для lint/template. Теги вида `gateway-preview` демонстрационные:
это не опубликованные образы нашего приложения.
Команды не устанавливают ресурсы и не читают Secrets.
Без аргументов скрипт выводит манифесты всех четырёх сервисов.

Можно задать реальные параметры и увидеть готовый манифест:

```bash
DOCKERHUB_NAMESPACE=nok1o IMAGE_TAG=your-published-version \
  bash backend/deploy/application.sh template gateway
```

`image.namespace` — Docker Hub пользователь/организация.
`KUBE_NAMESPACE` — namespace Kubernetes, по умолчанию `library-system`.
Это разные понятия.

## Установка после публикации образов: следующий шаг

Сначала в Docker Hub должны появиться четыре образа одной версии с разными тегами:

```text
docker.io/nok1o/lab4_rsoi:library-<version>
docker.io/nok1o/lab4_rsoi:rating-<version>
docker.io/nok1o/lab4_rsoi:reservation-<version>
docker.io/nok1o/lab4_rsoi:gateway-<version>
```

Желательно использовать тег коммита, а не изменяемый `latest`.
Сборка и публикация описаны в `IMAGES.md` и выполняются через `images.sh`.
Для публичных репозиториев imagePullSecrets не нужны. Для приватных понадобится
отдельный registry Secret, указанный в `imagePullSecrets`.

Когда образы опубликованы, а infra готова:

```bash
DOCKERHUB_NAMESPACE=nok1o IMAGE_TAG=your-published-version \
  bash backend/deploy/application.sh deploy
```

Deploy требует явно указать namespace образов и тег; `preview` запрещён.
Используется текущий kube-context, либо явно заданный `KUBE_CONTEXT`.
Скрипт проверяет наличие двух Secrets, устанавливает downstream-сервисы,
затем Gateway и ждёт готовности каждого Deployment.
Наличие Secrets не означает, что БД уже инициализированы или образы опубликованы:
это нужно проверить до команды установки.
При ошибке уже обновлённые release не откатываются автоматически.

Для одного сервиса добавьте его короткое имя:

```bash
DOCKERHUB_NAMESPACE=nok1o IMAGE_TAG=your-published-version \
  bash backend/deploy/application.sh deploy rating
```

То же самое напрямую через Helm, без обёртки:

```bash
helm upgrade --install library-service backend/deploy/helm/service \
  --namespace library-system \
  --values backend/deploy/helm/values/library.yaml \
  --set-string image.namespace=nok1o \
  --set-string image.tag=your-published-version \
  --wait --timeout 10m
```

## Проверка после будущей установки

```bash
kubectl -n library-system get deployments,pods,svc,ingress
kubectl -n library-system logs deployment/gateway-service
```

Ожидаем четыре готовых Deployment и ровно один Ingress — `gateway-service`.
Library, Reservation, Rating, PostgreSQL и Redis не получают внешних адресов.
Проверять Gateway нужно через внешний IP существующего Traefik:

```bash
curl --fail http://<traefik-external-ip>/manage/health
```

Если Pod не запускается, начните с `kubectl describe pod` и `kubectl logs`.
`ImagePullBackOff` обычно требует проверки имени/тега образа и доступа к registry.
Если Gateway отвечает, но API возвращает ошибки, смотрите логи соответствующего
сервиса и проверьте БД: успешная health-проверка не доказывает работу бизнес-операций.
