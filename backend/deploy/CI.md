# Полный облачный прогон ЛР4

Команды выполняет владелец проекта из корня lab4_rsoi. Workflow не создаёт
кластер, IAM-аккаунты, федерации, RBAC, PostgreSQL, Redis или их пароли.
Существующая инфраструктура в namespace library-system должна быть готова.
Корневой README и исходная Postman-коллекция не изменяются.

## Что делает workflow

Для pull request в master: Go build, unit-тесты с race detector, go vet,
проверка shell-скриптов, mock-тесты оркестратора и lint двух Helm charts.
В этом job нет облачной аутентификации.

Для push в master и ручного запуска из master после успешных проверок:

1. Собираются четыре linux/amd64 образа и публикуются в nok1o/lab4_rsoi.
2. Версия содержит SHA, run ID и номер попытки. При повторном запуске получаем
   новые теги, а не перезаписываем уже использованную версию.
3. GitHub OIDC-токен обменивается на временный IAM-токен отдельного CI-аккаунта.
4. На runner создаётся временный kubeconfig с проверкой CA кластера.
5. Проверяются identity, доступ к приложению и отсутствие опасных прав.
6. Helm обновляет Library, Rating, Reservation, Gateway и ждёт их готовности.
7. GitHub Classroom запускает success и все четыре этапа failover через Newman.
   HTTP-запросы идут к публичному Ingress, kubectl — к API облачного кластера.
8. Восстанавливается исходное число реплик Rating. При ошибке сохраняются логи
   диагностики, JSON-отчёты Newman загружаются как artifact на семь дней.
9. Временный kubeconfig удаляется. В artifacts он не попадает.

Группа concurrency общая для всего namespace; cancel-in-progress выключен.
Два workflow этого репозитория не будут одновременно выключать один Rating.
Ручные вмешательства или другой репозиторий эта настройка не блокирует.

## Почему два вида аккаунтов — не одно и то же

Создаём отдельный **Yandex IAM service account**, не Kubernetes ServiceAccount.
Он не должен совпадать с аккаунтом кластера или нод.
GitHub получает его IAM-токен через федерацию, а Kubernetes RoleBinding связывает
сырой IAM ID с ролью внутри library-system.

Файл kubernetes/github-actions-role.yaml разрешает деплой Deployment, Service,
Ingress и управление Helm-релизами, чтение Pods, EndpointSlice и диагностики.
Право на deployments/scale явно выдано только для rating-service.
При этом это **доверенный deployer**, не изолированный test-only аккаунт:
изменение обычного Deployment тоже позволяет менять replicas и запускать код.
Helm хранит свои release в Secrets, поэтому аккаунт может читать и менять
Secrets этого namespace, включая пароли БД/Redis.

Нет прав создавать RoleBinding/ClusterRoleBinding, менять StatefulSet/PVC,
ноды или ресурсы других namespace. Нет постоянного Kubernetes-токена.
Права приложения не ограничивают возможности вредоносного кода внутри
доверенного namespace: защищайте master и проверяйте изменения workflow.

## 1. Создать отдельный IAM-аккаунт

Эти команды создают ресурсы в Yandex Cloud. Выполните их один раз:

~~~bash
ci_sa_id="$(yc iam service-account create \
  --endpoint api.cloud.yandex.net:443 \
  --folder-id b1gs17mrf2d664pfdqsm \
  --name rsoi-lab4-ci \
  --description "GitHub Actions deployer for lab4 library-system" \
  --format json | jq --raw-output '.id')"
echo "$ci_sa_id"
~~~

Сохраните ID: это не секрет. Если аккаунт уже существует, не создавайте второй:
получите его ID через yc iam service-account get с тем же именем и folder-id.
Не создавайте авторизованный ключ, API key или статический access key.

Мы обращаемся прямо к известному Kubernetes endpoint с CA; CI не вызывает
Yandex API для получения метаданных кластера и не запускает yc get-credentials.
Не выдавайте на всякий случай k8s.cluster-api.editor/viewer/cluster-admin:
эти роли дают доступ к Kubernetes во всех namespace.
Доступ нашего аккаунта проверяется первым реальным запуском через auth whoami
и can-i; ошибка доступа — повод проверить subject/RoleBinding, не расширять права.

## 2. Настроить доверие GitHub → Yandex Cloud

~~~bash
federation_id="$(yc iam workload-identity oidc federation create \
  --endpoint api.cloud.yandex.net:443 \
  --folder-id b1gs17mrf2d664pfdqsm \
  --name rsoi-lab4-github \
  --issuer https://token.actions.githubusercontent.com \
  --audiences https://github.com/Nok1o \
  --jwks-url https://token.actions.githubusercontent.com/.well-known/jwks \
  --format json | jq --raw-output '.id')"

yc iam workload-identity federated-credential create \
  --endpoint api.cloud.yandex.net:443 \
  --folder-id b1gs17mrf2d664pfdqsm \
  --service-account-id "$ci_sa_id" \
  --federation-id "$federation_id" \
  --external-subject-id "repo:Nok1o/lab4_rsoi:ref:refs/heads/master"
~~~

Issuer — кто подписывает GitHub JWT; JWKS — где Yandex проверяет подпись.
Audience должен совпадать с https://github.com/<RepositoryOwner>: именно такое
значение использует yc-actions/yc-iam-token-fed. Сохраняйте регистр Nok1o.
Subject разрешает только этот репозиторий и ветку master, не fork и не PR.

В workflow намеренно нет GitHub Environment. Если добавить environment,
GitHub изменит subject на repo:Nok1o/lab4_rsoi:environment:<name>:
нужно будет отдельно изменить доверие федерации и настроить защиту Environment.

## 3. Выдать Kubernetes-права на приложение

Команды выполняются с вашим администраторским контекстом, не от имени CI:

~~~bash
kubectl --context rsoi-lab4 apply \
  -f backend/deploy/kubernetes/github-actions-role.yaml

kubectl --context rsoi-lab4 -n library-system create rolebinding github-actions-app-deployer \
  --role=github-actions-app-deployer \
  --user="$ci_sa_id" \
  --dry-run=client -o yaml |
  kubectl --context rsoi-lab4 apply -f -
~~~

В subjects будет kind: User и name: <сырой IAM ID>. Это не
serviceAccount:<ID> и не kind: ServiceAccount.
Команды не дают CI прав менять саму роль или binding.

Проверка RBAC вашим администратором с impersonation:

~~~bash
kubectl --context rsoi-lab4 --as="$ci_sa_id" -n library-system \
  auth can-i patch deployments/rating-service --subresource=scale

kubectl --context rsoi-lab4 --as="$ci_sa_id" -n kube-system \
  auth can-i get secrets

kubectl --context rsoi-lab4 --as="$ci_sa_id" \
  auth can-i create clusterrolebindings
~~~

Ожидаем yes, no, no. Ответ no завершает отдельную команду кодом 1 — это нормально.
Impersonation проверяет RBAC, но не OIDC-аутентификацию:
полную связку проверит первый workflow через auth whoami.

## 4. Настроить GitHub Variables и Secrets

Откройте Nok1o/lab4_rsoi → Settings → Secrets and variables → Actions.

В **Variables** добавьте:

| Имя | Значение |
| --- | --- |
| YC_SERVICE_ACCOUNT_ID | ID отдельного rsoi-lab4-ci |
| APP_BASE_URL | Публичный Gateway Ingress URL, сейчас http://51.250.9.181 |
| DOCKERHUB_NAMESPACE | nok1o |
| KUBE_CLUSTER_JSON | JSON из команды ниже |

~~~bash
KUBE_CONTEXT=rsoi-lab4 bash backend/deploy/export-cluster-config.sh
~~~

Скопируйте весь JSON в значение KUBE_CLUSTER_JSON. Скрипт читает локальный
kubeconfig и выводит **только server и certificate-authority-data**.
CA-сертификат и адрес публичные; users, admin-токены, exec-плагины и ключи
не экспортируются. Не копируйте вместо этого весь свой kubeconfig.
Endpoint должен быть внешним HTTPS-адресом API, доступным GitHub runner.
Если кластер пересоздан, обновите JSON; если поменялся Ingress IP — APP_BASE_URL.

В **Secrets** добавьте только:

| Имя | Значение |
| --- | --- |
| DOCKERHUB_TOKEN | Docker Hub access token с Read & Write для публикации образов |

Создайте токен в настройках аккаунта Docker Hub и не отправляйте его в чат.
Пароль Docker Hub, Google token и постоянный ключ Yandex Cloud этому workflow
не нужны.

Если используете установленный GitHub CLI, вместо ручного заполнения:

~~~bash
gh variable set YC_SERVICE_ACCOUNT_ID --repo Nok1o/lab4_rsoi --body "$ci_sa_id"
gh variable set APP_BASE_URL --repo Nok1o/lab4_rsoi --body "http://51.250.9.181"
gh variable set DOCKERHUB_NAMESPACE --repo Nok1o/lab4_rsoi --body nok1o
gh variable set KUBE_CLUSTER_JSON --repo Nok1o/lab4_rsoi \
  --body "$(KUBE_CONTEXT=rsoi-lab4 bash backend/deploy/export-cluster-config.sh)"
gh secret set DOCKERHUB_TOKEN --repo Nok1o/lab4_rsoi
~~~

Последняя команда интерактивно запрашивает секрет, не помещайте его в аргументы.

## 5. Первый запуск и диагностика

После сохранения и публикации изменений запустите GitHub Classroom Workflow
из Actions → Run workflow → master. Следующие push в master запускают его сами.
Workflow временно выключает настоящий облачный Rating; не запускайте в это
время ручной Newman против той же тестовой книги.

На шаге проверки identity:

- auth whoami должен показать IAM-аккаунт, связанный с RoleBinding;
- разрешение scale должно дать yes;
- доступ к Secrets kube-system и создание ClusterRoleBinding — no.

Если IAM token exchange не проходит, проверьте audience, subject, имя owner,
ветку master и federated credential. Если Kubernetes отвечает 403 — проверьте
identity и RoleBinding. Если i/o timeout — проверьте маршрутизацию/SG, а не RBAC.
Runner должен иметь доступ к Kubernetes API по HTTPS и Gateway Ingress.
Workflow не открывает SG на 0.0.0.0/0 автоматически. При закрытом API используйте
runner с подходящим сетевым доступом или согласованное правило доступа.

Secret PostgreSQL/Redis, PVC, данные и инфраструктурный Helm release не
пересоздаются. При обычной ошибке тестов сам скрипт восстанавливает Rating,
дополнительный шаг с always() повторяет восстановление в workflow.
При потере runner/сети или принудительном завершении гарантировать cleanup
невозможно: вручную проверьте replicas и историю тестового пользователя.
Не включайте cancel-in-progress: true.

Ошибка тестов делает job красным: у Autograding нет continue-on-error.
Права GitHub actions: read и checks: write нужны для публикации баллов Classroom,
они не дают прав на Kubernetes. Отката версии приложения при ошибке тестов нет.

## Повторяемость тестов

Скрипт scripts/test-script.sh:

- проверяет доступность seed-книги до начала; при чужой активной аренде
  останавливается, не отменяя её и не сбрасывая БД;
- создаёт уникальное имя CI-пользователя, чтобы не достичь потолка рейтинга 100;
- использует временные копии collection/environment, меняя только пользователя
  и baseUrl, а не исходные тестовые assertions;
- ждёт исчезновения Pods/EndpointSlice при остановке Rating и готовности
  Deployment/EndpointSlice при восстановлении;
- выполняет success → step1 → step2 → step3 → step4;
- в EXIT trap возвращает незавершённые аренды только своего пользователя
  и только seed-книги, даже если assertion не успел сохранить reservationUid.

История тестовых пользователей и завершённых аренд остаётся в БД.
Это повторяемые тесты с изолированным пользователем и cleanup, не идемпотентность
бизнес-операций. Общая seed-книга всё ещё требует последовательных запусков.
При аварии имя пользователя есть в отчёте context.json и в логе скрипта.

Для ручного полного прогона из своего терминала нужен установленный Newman:

~~~bash
npm install --global newman@6.2.1
BASE_URL=http://51.250.9.181 KUBE_CONTEXT=rsoi-lab4 \
  bash scripts/test-script.sh v4 rating-service library-system
~~~

Эта команда действительно отключает Rating в облаке и изменяет тестовые данные.
Безопасная локальная проверка самого оркестратора, без сети:

~~~bash
bash scripts/test-script-test.sh
~~~

## Источники схемы доступа

- [OIDC-интеграция GitHub и Yandex Cloud](https://yandex.cloud/en/docs/tutorials/security/wlif-github-integration).
- [Action для обмена токена](https://github.com/yc-actions/yc-iam-token-fed).
- [Разграничение доступа Managed Kubernetes](https://yandex.cloud/en/docs/managed-kubernetes/security/).
- [Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/).
