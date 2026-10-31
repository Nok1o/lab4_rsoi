# Сборка и деплой ЛР4

Команды из корня репозитория. Нужны Docker + buildx, Helm, kubectl, AWS CLI v2,
jq, openssl; для настройки GitHub — gh. EKS, EBS CSI, Traefik/NLB и StorageClass
gp3 уже созданы. Наружу публикуется только Gateway через Ingress.

## Окружение

~~~bash
export AWS_REGION=us-east-2
export EKS_CLUSTER_NAME=rsoi-lab4-aws
export KUBE_CONTEXT=rsoi-lab4-aws
export KUBE_NAMESPACE=library-system
export DOCKERHUB_NAMESPACE=nok1o
export DOCKERHUB_REPOSITORY=lab4_rsoi
aws sts get-caller-identity  # действующая локальная AWS-сессия
aws eks update-kubeconfig --region "$AWS_REGION" --name "$EKS_CLUSTER_NAME" \
  --alias "$KUBE_CONTEXT"
kubectl config use-context "$KUBE_CONTEXT"
kubectl get nodes
export BASE_URL="http://$(kubectl -n ingress-system get service traefik \
  -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')"
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
Не удалять PVC без резервной копии. gp3 использует Retain: оставшиеся EBS-диски
оплачиваются и после удаления PVC/кластера.

## GitHub Actions: однократная настройка

Сборка/push работают на GitHub-hosted runner, Helm и success/failover — в новом
одноразовом Pod ARC внутри EKS. AWS OIDC, access entry и AWS-ключи не нужны.
Публичный API кластера можно оставить доступным только с твоего IP.

Перед установкой: защитить master; Settings → Actions → General → Fork pull
request workflows → Require approval for all external contributors. Не одобрять
непроверенные изменения workflow. GitHub не рекомендует self-hosted runners для
публичных репозиториев: if/labels в нашем workflow не являются изоляцией от
другого workflow, добавленного в PR. Эта конфигурация — для короткого доверенного
учебного стенда; Pod имеет deploy-доступ и может читать Secrets library-system.

Создать fine-grained PAT: https://github.com/settings/personal-access-tokens/new.
Resource owner Nok1o; Only select repositories → lab4_rsoi; срок 7 дней;
Repository permissions → Administration: Read and write (Metadata — автоматически).
Токен нужен ARC для регистрации runner, не передаётся в jobs и GitHub Secrets.

~~~bash
kubectl --context "$KUBE_CONTEXT" create namespace arc-system --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -
kubectl --context "$KUBE_CONTEXT" create namespace arc-runners --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -

# Скрытый ввод PAT; без токена в истории команд, аргументах и локальных файлах.
bash -euo pipefail -c '
  read -r -s -p "GitHub PAT: " arc_pat
  printf "\n"
  test -n "$arc_pat"
  printf "%s" "$arc_pat" |
    kubectl --context "$KUBE_CONTEXT" -n arc-runners create secret generic arc-github-credentials \
      --from-file=github_token=/dev/stdin --dry-run=client -o yaml |
    kubectl --context "$KUBE_CONTEXT" apply -f -
'

# Отдельный образ runner: собрать/push один раз, не при каждой сборке приложения.
docker buildx build --platform linux/amd64 \
  --file backend/deploy/docker/ci-runner/Dockerfile \
  --tag docker.io/nok1o/lab4_rsoi:ci-runner-v1 --load backend
docker push docker.io/nok1o/lab4_rsoi:ci-runner-v1

bash backend/deploy/runner.sh deploy
kubectl --context "$KUBE_CONTEXT" -n arc-system get pods
kubectl --context "$KUBE_CONTEXT" -n arc-runners get pods,autoscalingrunnersets
~~~

runner.sh устанавливает официальный ARC controller и scale set версии 0.15.1.
Values: backend/deploy/runner/*.yaml. minRunners=0, maxRunners=1: в простое
работает listener; runner создаётся при появлении job lab4-cloud-runner.
Образ содержит kubectl 1.37.1, Helm 4.3.1, Node 24 и Newman 6.2.1.
В runner нет Docker daemon, privileged-режима, hostPath или постоянного диска.
Для новой версии образа использовать новый tag и обновить scale-set-values.yaml.

ServiceAccount arc-runners/github-actions-app-deployer получает Role только
в library-system. Его token автоматически ротируется Kubernetes; kubeconfig
использует tokenFile. Jobs не получают прав на Secrets arc-runners/kube-system
или RBAC. Администратор устанавливает ARC/RBAC вручную; CI их не меняет.

~~~bash
ci_user=system:serviceaccount:arc-runners:github-actions-app-deployer
kubectl --context "$KUBE_CONTEXT" --as="$ci_user" -n library-system \
  auth can-i patch deployments/rating-service --subresource=scale  # yes
kubectl --context "$KUBE_CONTEXT" --as="$ci_user" -n arc-runners auth can-i get secrets  # no

gh variable set APP_BASE_URL --repo Nok1o/lab4_rsoi --body "$BASE_URL"
gh variable set DOCKERHUB_NAMESPACE --repo Nok1o/lab4_rsoi --body "$DOCKERHUB_NAMESPACE"
gh secret set DOCKERHUB_TOKEN --repo Nok1o/lab4_rsoi  # если ещё не задан
~~~

DOCKERHUB_TOKEN — Docker Hub access token с Read & Write, отдельный от PAT для ARC.
Старые Actions Variables AWS_ROLE_ARN, EKS_CLUSTER_NAME, AWS_REGION,
YC_SERVICE_ACCOUNT_ID и KUBE_CLUSTER_JSON больше не используются.

Push в master: проверки → build/push четырёх образов → Helm deploy → success/failover.
PR: только локальные проверки. После публикации workflow:

~~~bash
gh workflow run classroom.yml --repo Nok1o/lab4_rsoi --ref master
gh run list --repo Nok1o/lab4_rsoi --workflow classroom.yml --limit 3
~~~

Если cloud job ожидает runner: проверить listener/controller logs и срок PAT.
Для замены истёкшего PAT повторить скрытый ввод Secret, затем runner.sh deploy.
После сдачи: helm uninstall lab4-cloud-runner -n arc-runners, затем
helm uninstall arc -n arc-system; отозвать PAT. Инфраструктуру и PVC это не удаляет.

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
Перед позитивными failover-этапами ждёт успешного чтения рейтинга через Gateway.
При обрыве runner/сети проверить replicas вручную; отчёты — в artifacts workflow.

Источники: [ARC](https://docs.github.com/en/actions/how-tos/manage-runners/use-actions-runner-controller/deploy-runner-scale-sets),
[GitHub PAT для ARC](https://docs.github.com/en/actions/how-tos/manage-runners/use-actions-runner-controller/authenticate-to-the-api),
[ServiceAccount](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/).
