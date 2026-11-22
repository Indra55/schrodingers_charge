set -eu

db_dir=${DEMO_DB_DIR:-.}
provider_port=${DEMO_PROVIDER_PORT:-8081}
mkdir -p "$db_dir"

provider_pid=
merchant_pid=
cleanup() {
    if [ -n "$merchant_pid" ]; then
        kill "$merchant_pid" 2>/dev/null || true
    fi
    if [ -n "$provider_pid" ]; then
        kill "$provider_pid" 2>/dev/null || true
    fi
    wait
}
trap cleanup EXIT
trap 'exit 143' TERM
trap 'exit 130' INT

./bin/provider -addr "127.0.0.1:$provider_port" -db "$db_dir/provider.db" -demo-faults &
provider_pid=$!
./bin/merchant -addr "0.0.0.0:${PORT:-10000}" -db "$db_dir/merchant.db" \
    -provider "http://127.0.0.1:$provider_port" serve &
merchant_pid=$!

wait -n "$provider_pid" "$merchant_pid"
