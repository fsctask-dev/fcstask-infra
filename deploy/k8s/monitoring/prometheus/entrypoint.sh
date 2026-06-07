set -eu

: "${GRAFANA_CLOUD_REMOTE_WRITE_URL:?GRAFANA_CLOUD_REMOTE_WRITE_URL is not set}"
: "${GRAFANA_CLOUD_REMOTE_WRITE_USERNAME:?GRAFANA_CLOUD_REMOTE_WRITE_USERNAME is not set}"
: "${GRAFANA_CLOUD_REMOTE_WRITE_PASSWORD:?GRAFANA_CLOUD_REMOTE_WRITE_PASSWORD is not set}"

TMPL=/etc/prometheus/prometheus.yml.tmpl
OUT=/tmp/prometheus.yml

sed \
  -e "s|\${GRAFANA_CLOUD_REMOTE_WRITE_URL}|${GRAFANA_CLOUD_REMOTE_WRITE_URL}|g" \
  -e "s|\${GRAFANA_CLOUD_REMOTE_WRITE_USERNAME}|${GRAFANA_CLOUD_REMOTE_WRITE_USERNAME}|g" \
  -e "s#\${GRAFANA_CLOUD_REMOTE_WRITE_PASSWORD}#${GRAFANA_CLOUD_REMOTE_WRITE_PASSWORD}#g" \
  "$TMPL" > "$OUT"

if [ ! -s "$OUT" ]; then
  echo "FATAL: rendered config $OUT is empty" >&2
  exit 1
fi

exec /bin/prometheus \
  --config.file="$OUT" \
  --storage.tsdb.path=/prometheus \
  --storage.tsdb.retention.time=7d \
  --storage.tsdb.retention.size=5GB \
  --web.enable-lifecycle
