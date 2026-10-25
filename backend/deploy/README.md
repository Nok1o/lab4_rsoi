# Сборка и деплой ЛР4

Команды из корня репозитория. Нужны Docker + buildx, Helm, kubectl, yc, jq,
openssl; для настройки GitHub — gh. Кластер, Traefik и StorageClass
yc-network-ssd уже созданы. Наружу публикуется только Gateway через Ingress.

## Окружение

~~~bash
export KUBE_CONTEXT=rsoi-lab4
export KUBE_NAMESPACE=library-system
export DOCKERHUB_NAMESPACE=nok1o
export DOCKERHUB_REPOSITORY=lab4_rsoi
export BASE_URL=http://51.250.9.181  # обновить, если IP Traefik изменился

yc managed-kubernetes cluster get-credentials catk60b05gm9nsj0gman \
  --external --context-name "$KUBE_CONTEXT" --endpoint api.cloud.yandex.net:443
kubectl config use-context "$KUBE_CONTEXT"
kubectl get nodes
~~~

## Сборка → push → деплой

Инфраструктура должна быть установлена — при первом запуске см. следующий раздел.
Для каждой новой сборки используем новый IMAGE_TAG.

~~~bash
colima start  # на Mac, если Docker ещё не запущен
docker buildx version
docker login --username "$DOCKERHUB_NAMESPACE"  # пароль: Docker Hub access token
export IMAGE_TAG="manual-$(date -u +%Y%m%d%H%M%S)"

bash backend/deploy/images.sh build
bash backend/deploy/images.sh push
bash backend/deploy/application.sh deploy
curl --fail "$BASE_URL/manage/health"
~~~

Образы: nok1o/lab4_rsoi:<library|rating|reservation|gateway>-$IMAGE_TAG,
платформа linux/amd64. Values: backend/deploy/helm/values/*.yaml.
Для одного сервиса добавляем library, rating, reservation или gateway
последним аргументом скрипта. Для повторного деплоя готовых образов — только
application.sh deploy с их IMAGE_TAG.

## Инфраструктура: один раз на новом namespace

~~~bash
kubectl create namespace "$KUBE_NAMESPACE"
kubectl -n "$KUBE_NAMESPACE" create secret generic postgresql-credentials \
  --from-literal=POSTGRES_PASSWORD="$(openssl rand -hex 24)" \
  --from-literal=LIBRARIES_DB_PASSWORD="$(openssl rand -hex 24)" \
  --from-literal=RESERVATIONS_DB_PASSWORD="$(openssl rand -hex 24)" \
  --from-literal=RATINGS_DB_PASSWORD="$(openssl rand -hex 24)"
kubectl -n "$KUBE_NAMESPACE" create secret generic redis-credentials \
  --from-literal=REDIS_PASSWORD="$(openssl rand -hex 24)"
bash backend/deploy/infrastructure.sh deploy
~~~

Существующие Secrets не пересоздавать. SQL из postgres/scripts/schema-*.sql
применяется только при первом запуске PostgreSQL на пустом диске.
Не удалять PVC: можно потерять данные и облачный диск.

## GitHub Actions: однократная настройка

IAM и OIDC уже созданы:

- CI service account: ajen3g2nl7o039bakoq6.
- Федерация: ajejtmjom45icm85fnc3.
- Доверенный subject: repo:Nok1o/lab4_rsoi:ref:refs/heads/master.

Выдать аккаунту роль только в library-system (администраторским контекстом):

~~~bash
ci_sa_id=ajen3g2nl7o039bakoq6
kubectl apply -f backend/deploy/kubernetes/github-actions-role.yaml
kubectl -n "$KUBE_NAMESPACE" create rolebinding github-actions-app-deployer \
  --role=github-actions-app-deployer --user="$ci_sa_id" --dry-run=client -o yaml |
  kubectl apply -f -

kubectl --as="$ci_sa_id" -n "$KUBE_NAMESPACE" \
  auth can-i patch deployments/rating-service --subresource=scale
kubectl --as="$ci_sa_id" -n kube-system auth can-i get secrets
~~~

Ожидаем yes, no. CI — доверенный deployer: ему доступны Secrets этого namespace.
Защитить master; GitHub runner должен иметь сетевой доступ к API кластера и Ingress.

Заполнить Actions Variables/Secrets через gh (или Settings → Secrets and variables → Actions):

~~~bash
gh variable set YC_SERVICE_ACCOUNT_ID --repo Nok1o/lab4_rsoi --body "$ci_sa_id"
gh variable set APP_BASE_URL --repo Nok1o/lab4_rsoi --body "$BASE_URL"
gh variable set DOCKERHUB_NAMESPACE --repo Nok1o/lab4_rsoi --body "$DOCKERHUB_NAMESPACE"
gh variable set KUBE_CLUSTER_JSON --repo Nok1o/lab4_rsoi \
  --body "$(bash backend/deploy/export-cluster-config.sh)"
gh secret set DOCKERHUB_TOKEN --repo Nok1o/lab4_rsoi
~~~

DOCKERHUB_TOKEN — Docker Hub access token с Read & Write; вводится интерактивно.
Постоянный ключ Yandex не нужен: workflow получает временный токен через OIDC.
При пересоздании кластера обновить KUBE_CLUSTER_JSON.

Push в master: проверки → build/push образов → Helm deploy → success/failover.
PR: только локальные проверки. Ручной запуск после публикации изменений:

~~~bash
gh workflow run classroom.yml --repo Nok1o/lab4_rsoi --ref master
~~~

## Тесты и диагностика

~~~bash
(cd backend && go test -race ./...)
bash scripts/test-script-test.sh  # mock-тесты, без облака
bash backend/deploy/infrastructure.sh lint
bash backend/deploy/application.sh lint

npm install --global newman@6.2.1
bash scripts/test-script.sh v4 rating-service "$KUBE_NAMESPACE"

kubectl -n "$KUBE_NAMESPACE" get pods,pvc,svc,ingress
kubectl -n "$KUBE_NAMESPACE" logs deployment/gateway-service --tail=100
kubectl -n "$KUBE_NAMESPACE" get events --sort-by=.lastTimestamp
~~~

Полный Newman-прогон временно выключает облачный Rating и меняет тестовые данные.
Не запускать параллельно с CI; seed-книга должна быть свободна и в EXCELLENT.
Скрипт восстанавливает Rating и возвращает незавершённые аренды своего CI-пользователя.
При обрыве runner/сети проверить replicas вручную; отчёты — в artifacts workflow.
