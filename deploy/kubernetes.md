# Kubernetes в этом репозитории

Краткий гайд по работе с кластером и деплоем PostgreSQL (Bitnami Helm chart).

## Что понадобится

- **kubectl** настроен на нужный контекст (`kubectl config current-context`).
- **Helm 3.8+** для установки чарта из OCI.

Проверка:

```bash
kubectl cluster-info
kubectl get nodes
helm version
```

## PostgreSQL (репликация)

Конфигурация чарта лежит в [`k8s/postgres/values.yaml`](k8s/postgres/values.yaml): один **primary** (запись) и **две read-replica** (чтение).

Установка или обновление релиза (namespace создаётся при необходимости):

```bash
helm upgrade --install fcstask-pg oci://registry-1.docker.io/bitnamicharts/postgresql \
  --version 18.6.2 \
  -n fcstask-db --create-namespace \
  -f deploy/k8s/postgres/values.yaml
```

Удаление релиза:

```bash
helm uninstall fcstask-pg -n fcstask-db
```

После удаления через Helm PVC могут остаться (и со старым паролем данных). Явно удалите PVC/PV, если нужна «чистая» установка.

### Распределение по узлам

В [`k8s/postgres/values.yaml`](k8s/postgres/values.yaml) заданы **topology spread** по `kubernetes.io/hostname` для всех подов релиза `fcstask-pg` с именем `postgresql`, а для read-replica дополнительно **`podAntiAffinityPreset: hard`**, чтобы реплики не садились на один узел друг с другом.

Имеет смысл только если в кластере **не меньше двух узлов, куда scheduler реально кладёт поды** (нет `NoSchedule` на всех воркерах кроме одного). Имя релиза в чарте должно совпадать с `app.kubernetes.io/instance` в `topologySpreadConstraints` (сейчас **`fcstask-pg`**); при другом имени релиза поправьте лейблы в `values.yaml`.

После смены правил уже запущенные поды могут остаться на старых узлах; чтобы перераспределить, пересоздайте поды (например, **`kubectl rollout restart statefulset/... -n fcstask-db`** или удаление конкретного Pod).

### Хранилище

Подам нужны динамические тома. Должен быть **StorageClass** (часто с аннотацией default). Если PVC висят в `Pending` и в событиях «no storage class is set», задайте класс в `values.yaml` (`primary.persistence.storageClass`, `readReplicas.persistence.storageClass`) или поставьте провайдер и default StorageClass для кластера.

### Проверка состояния

Список подов и PVC:

```bash
kubectl get pods,pvc -n fcstask-db
```

Ожидание готовности подов без ручных пауз:

```bash
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/instance=fcstask-pg -n fcstask-db --timeout=300s
```

Статус выката StatefulSet:

```bash
kubectl rollout status statefulset/fcstask-pg-postgresql-primary -n fcstask-db --timeout=300s
kubectl rollout status statefulset/fcstask-pg-postgresql-read -n fcstask-db --timeout=300s
```

### Подключение к базе

DNS **внутри кластера** (запись и чтение):

- Запись: `fcstask-pg-postgresql-primary.fcstask-db.svc.cluster.local:5432`
- Чтение (реплики за сервисом): `fcstask-pg-postgresql-read.fcstask-db.svc.cluster.local:5432`

Пароль пользователя `postgres` из секрета:

```bash
kubectl get secret -n fcstask-db fcstask-pg-postgresql \
  -o jsonpath='{.data.postgres-password}' | base64 -d
echo
```

Доступ **с машины разработчика** через port-forward:

```bash
kubectl port-forward -n fcstask-db svc/fcstask-pg-postgresql-primary 5432:5432
```

Дальше подключайтесь к `127.0.0.1:5432` с тем же пользователем и БД из `values.yaml`.

### Полезное при сбоях

- **`kubectl describe pod <имя> -n fcstask-db`** и **`kubectl logs <имя> -n fcstask-db`** — причины `CrashLoopBackOff` или зависших readiness.
- Реплики чарта поднимаются по очереди: пока не готов `read-0`, второй реплика-под может не появиться.
- Если у **read** в логах `Waiting for replication master... primary:5432 - no response`, а primary уже **Running**, часто виноват **NetworkPolicy** чарта: правило ingress на primary не учитывает поды с `component: read`. В [`k8s/postgres/values.yaml`](k8s/postgres/values.yaml) для этого отключены `primary.networkPolicy` и `readReplicas.networkPolicy`; после правки выполните `helm upgrade ...` с тем же `-f`.

## Docker Compose

Локальный вариант без Kubernetes описан в [`postgres-replication/docker-compose.yml`](postgres-replication/docker-compose.yml); для прод-подобного стека в кластере используйте Helm и `values.yaml` выше.

## Monitoring (Prometheus + Alertmanager)

Helm chart: [`helm/fcstask-monitoring/`](../helm/fcstask-monitoring/). Конфиги-источники лежат в [`k8s/monitoring/`](k8s/monitoring/).

Prometheus скрейпит `/metrics` с fcstask-backend (`fcsapp:8081`) и пишет метрики в Grafana Cloud через `remote_write`. Alertmanager отправляет алерты в Telegram bot.

**Namespaces:**

- test: `fcstask-monitoring-test` (деплой с ветки `main` через CD)
- prod: `fcstask-monitoring` (деплой с веток `release/**` через CD)

### GitHub Secrets (репозиторий fcstask-infra)

| Secret | Назначение |
|--------|------------|
| `KUBECONFIG` | kubeconfig кластера |
| `GRAFANA_CLOUD_REMOTE_WRITE_URL` | URL remote_write |
| `GRAFANA_CLOUD_REMOTE_WRITE_USERNAME` | basic auth user ID |
| `GRAFANA_CLOUD_REMOTE_WRITE_PASSWORD` | token с `metrics:write` |

### Ручная установка

```bash
helm upgrade --install fcstask-monitoring ./helm/fcstask-monitoring \
  -n fcstask-monitoring --create-namespace \
  --set global.cluster=fcstask-prod \
  --set global.env=prod \
  --set scrape.backend.host=fcsapp.fcstask-backend.svc.cluster.local \
  --set grafanaCloud.remoteWriteUrl="$GRAFANA_CLOUD_REMOTE_WRITE_URL" \
  --set grafanaCloud.remoteWriteUsername="$GRAFANA_CLOUD_REMOTE_WRITE_USERNAME" \
  --set-string grafanaCloud.remoteWritePassword="$GRAFANA_CLOUD_REMOTE_WRITE_PASSWORD" \
  --wait
```

### Проверка

```bash
kubectl get pods -n fcstask-monitoring
kubectl port-forward -n fcstask-monitoring svc/fcstask-monitoring-prometheus 9090:9090
```

В Prometheus UI (`http://localhost:9090`) проверьте target `fcstask-backend` — статус **UP**. В Grafana Cloud должны появиться метрики `fcstask_*`.

**Prerequisite:** backend Helm chart должен экспонировать metrics port `:8081` на Service `fcsapp` (репозиторий fcstask-backend).
