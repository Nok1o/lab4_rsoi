# Шаг 1. PostgreSQL и Redis

Все команды ниже запускаются из корня `lab4_rsoi`.
Корневой README, исходные SQL-схемы и код Go-сервисов не изменяются.

## Что создаёт chart

Release `library-infra` в namespace `library-system`:

| Компонент | Объекты | Данные | Внутренний адрес |
| --- | --- | --- | --- |
| PostgreSQL 17 | StatefulSet, ConfigMap, Service | PVC 10 GiB | `library-infra-postgresql:5432` |
| Redis 7.4 | StatefulSet, ConfigMap, Service | PVC 1 GiB | `library-infra-redis:6379` |

У каждого StatefulSet дополнительно есть headless Service для стабильного DNS-имени
Pod. Клиенты используют обычные Service из таблицы.
Все Service имеют тип `ClusterIP`: внешнего IP, `NodePort` и `Ingress` у баз нет.
Это не изоляция от других Pod кластера: доступ дополнительно проверяется паролями.

StatefulSet сохраняет связь экземпляра с его PVC при замене Pod.
Сам диск выделяет CSI-драйвер Yandex Cloud по запросу PVC через класс
`yc-network-ssd`. PostgreSQL и Redis работают в одном экземпляре: это не HA.
При потере ноды возможна пауза, пока Pod будет перенесён и диск подключён заново.
Размеры и requests/limits заданы в `helm/infrastructure/values.yaml`.
11 GiB для данных выделяются дополнительно к загрузочным дискам рабочих нод
и расходуют квоту и бюджет облака.

## Инициализация PostgreSQL

Для нового кластера используем PostgreSQL 17. Старый локальный Compose с PostgreSQL 13
не меняем и существующий локальный диск сюда не подключаем.

При первом запуске на пустом диске официальный entrypoint исполняет
`files/postgresql/init.sql`, смонтированный в `/docker-entrypoint-initdb.d`.
Этот файл:

1. Читает пароли из переменных окружения через `psql \getenv`.
   Запись `:'variable'` экранирует значение как SQL-строку.
2. Создаёт три роли без прав суперпользователя и три БД.
3. Назначает каждой роли владение только её БД и убирает доступ `PUBLIC`
   к этим трём БД.
4. Подключается к каждой БД, переключается на её владельца и исполняет
   соответствующую существующую SQL-схему.

| Сервис | БД | Пользователь | Ключ пароля в Secret |
| --- | --- | --- | --- |
| Library | `libraries` | `library_service` | `LIBRARIES_DB_PASSWORD` |
| Reservation | `reservations` | `reservation_service` | `RESERVATIONS_DB_PASSWORD` |
| Rating | `ratings` | `rating_service` | `RATINGS_DB_PASSWORD` |

Суперпользователь `postgres` предназначен для администрирования, не для сервисов.
Схемы берутся из `postgres/scripts/schema-*.sql`. Скрипт `infrastructure.sh`
передаёт их в Helm через `--set-file`; ConfigMap доставляет их в Pod.
Вторых копий схем в репозитории нет. Исходные локальные init-скрипты остаются
без изменений; для Kubernetes используется отдельный bootstrap с тремя ролями.
В Go-коде инициализации/миграций нет.

Скрипты entrypoint выполняются **только при первом запуске на пустом PGDATA**.
`helm upgrade`, изменение ConfigMap или перезапуск Pod не применяют новую схему
и не меняют пароли существующих ролей. Такие изменения позже нужно выполнять
явными SQL-операциями. Если первоначальная инициализация прервалась, повторный
запуск на частично заполненном диске также не завершит её автоматически:
проверьте логи и состояние БД, не удаляйте PVC вслепую.

## Сохранность Redis

Redis хранит очередь Streams и pending-сообщения в `/data`, на PVC.
Включены AOF и `appendfsync everysec`: при аварийной потере процесса/ноды
возможно потерять примерно последнюю секунду записей. PVC и AOF не обеспечивают
exactly-once и не делают downstream-операции идемпотентными.

`maxmemory-policy noeviction` не позволяет Redis вытеснять сообщения очереди
при нехватке памяти: запись завершится ошибкой. При лимите памяти контейнера
512 MiB для данных Redis установлено `maxmemory 256mb`; остальное оставлено
под накладные расходы. Это начальные настройки для учебной нагрузки.
Доступ защищён паролем. Redis, как и PostgreSQL, не публикуется наружу.
TLS внутри этого учебного кластера пока не настраивается.

## Проверка без установки

```bash
bash backend/deploy/infrastructure.sh lint
bash backend/deploy/infrastructure.sh template
```

`lint` проверяет chart, `template` выводит готовые Kubernetes-манифесты.
Ни одна из этих команд не обращается к кластеру и не создаёт ресурсы.
Если вызвать скрипт без аргумента, используется безопасный режим `template`.

## Первое развёртывание

Следующие команды уже меняют кластер и создают платные диски.
Сначала проверьте текущий контекст и готовность обеих нод после включения OS Login:

```bash
kubectl config current-context
kubectl get nodes
```

Обе ноды должны быть `Ready`, без `SchedulingDisabled`; группа узлов Yandex Cloud
должна закончить `RECONCILING`.

Создайте namespace и два Secret **один раз**:

```bash
kubectl create namespace library-system

kubectl -n library-system create secret generic postgresql-credentials \
  --from-literal=POSTGRES_PASSWORD="$(openssl rand -hex 24)" \
  --from-literal=LIBRARIES_DB_PASSWORD="$(openssl rand -hex 24)" \
  --from-literal=RESERVATIONS_DB_PASSWORD="$(openssl rand -hex 24)" \
  --from-literal=RATINGS_DB_PASSWORD="$(openssl rand -hex 24)"

kubectl -n library-system create secret generic redis-credentials \
  --from-literal=REDIS_PASSWORD="$(openssl rand -hex 24)"
```

Пароли создаются локально, не выводятся и не сохраняются в Git или values-файлы.
Secret — не шифрование само по себе: доступ к нему ограничивается RBAC.
Ошибки `AlreadyExists` означают, что объект уже есть. Не удаляйте и не заменяйте
Secret новыми случайными значениями при повторном деплое: пароли на диске БД
от этого не изменятся. Нужна отдельная процедура ротации.
Для аварийного восстановления потребуется защищённая копия учётных данных.

Установите инфраструктуру:

```bash
bash backend/deploy/infrastructure.sh deploy
```

Скрипт использует текущий kube-context, проверяет наличие Secret и выполняет
`helm upgrade --install`. Можно явно указать контекст:

```bash
KUBE_CONTEXT=rsoi-lab4 bash backend/deploy/infrastructure.sh deploy
```

## Проверка результата

```bash
kubectl -n library-system get pods,pvc,svc
kubectl -n library-system logs library-infra-postgresql-0
kubectl -n library-system exec library-infra-postgresql-0 -- \
  psql -U postgres -d postgres -c '\l'
kubectl -n library-system exec library-infra-postgresql-0 -- \
  psql -U postgres -d libraries -c '\dt'
kubectl -n library-system exec library-infra-postgresql-0 -- \
  psql -U postgres -d reservations -c '\dt'
kubectl -n library-system exec library-infra-postgresql-0 -- \
  psql -U postgres -d ratings -c '\dt'
kubectl -n library-system exec library-infra-redis-0 -- redis-cli ping
```

Ожидаем оба Pod `1/1 Running`, оба PVC `Bound`, три БД с таблицами и ответ `PONG`.
`REDISCLI_AUTH` уже передаётся контейнеру из Secret, пароль в команде не нужен.
При `Pending` смотрите `kubectl describe pod ...` и `kubectl describe pvc ...`:
это покажет нехватку ресурсов, проблемы подключения или выделения диска.
`WaitForFirstConsumer` у StorageClass означает, что диск выделяется после
выбора ноды для Pod; первоначальный `Pending` PVC сам по себе не ошибка.

Для будущих сервисов используем `DB_HOST=library-infra-postgresql`, порт `5432`,
имя БД/пользователя из таблицы и `DB_PASSWORD` из нужного ключа Secret.
Gateway получит Redis URL с паролем из `redis-credentials`.
Существующие конфиги Go поддерживают эти параметры; менять код не требуется.

## Удаление и резервные копии

В StatefulSet явно задано сохранение PVC при удалении StatefulSet или уменьшении
числа реплик. Поэтому `helm uninstall` не удалит данные, а сохранённые диски
продолжат тарифицироваться.

Но **ручное удаление PVC опасно**: у класса `yc-network-ssd` политика `Delete`,
так что вместе с PVC может быть удалён и облачный диск. PVC — не резервная копия.
Здесь пока нет автоматического бэкапа или репликации.

Существующий Traefik и его балансировщик этот chart не изменяет.
Универсальный chart для четырёх Go-сервисов и Ingress Gateway — следующий шаг.
