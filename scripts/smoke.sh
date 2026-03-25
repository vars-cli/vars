#!/usr/bin/env bash
set -euo pipefail

BIN="${1:-./vars}"

export VARS_STORE_DIR=$(mktemp -d)
WORKDIR=$(mktemp -d)
trap "$BIN agent stop 2>/dev/null; rm -rf $VARS_STORE_DIR $WORKDIR" EXIT

contains() { echo "$1" | grep -q "$2"; }

echo "--- set keys (first run auto-creates store) ---"
echo -e "\n\n" | $BIN set RPC_URL https://rpc.example.com
$BIN set PRIVATE_KEY 0xTESTKEY
$BIN set ETHERSCAN_API abc123

echo "--- get ---"
test "$($BIN get RPC_URL)" = "https://rpc.example.com"

echo "--- ls ---"
test "$($BIN ls | wc -l)" -eq 3

echo "--- resolve (posix) ---"
cat > "$WORKDIR/.vars.yaml" <<'YAML'
keys:
  - RPC_URL
  - PRIVATE_KEY
YAML
eval "$($BIN resolve -f "$WORKDIR/.vars.yaml")"
test "$RPC_URL" = "https://rpc.example.com"
test "$PRIVATE_KEY" = "0xTESTKEY"

echo "--- resolve (fish) ---"
contains "$($BIN resolve -f "$WORKDIR/.vars.yaml" --format fish)" "set -x"

echo "--- resolve (dotenv) ---"
contains "$($BIN resolve -f "$WORKDIR/.vars.yaml" --format dotenv)" "RPC_URL="

echo "--- resolve --partial ---"
cat > "$WORKDIR/.vars.yaml" <<'YAML'
keys:
  - RPC_URL
  - MISSING_KEY
YAML
contains "$($BIN resolve -f "$WORKDIR/.vars.yaml" --partial 2>/dev/null)" "MISSING_KEY"

echo "--- dump ---"
contains "$($BIN dump --format dotenv 2>/dev/null)" "ETHERSCAN_API"

echo "--- history ---"
$BIN set --overwrite RPC_URL https://rpc-v2.example.com
$BIN set --overwrite RPC_URL https://rpc-v3.example.com
HIST=$($BIN history RPC_URL)
contains "$HIST" "RPC_URL~2:"
contains "$HIST" "https://rpc-v2.example.com"
contains "$HIST" "RPC_URL~1:"
contains "$HIST" "https://rpc.example.com"
test "$($BIN ls | wc -l)" -eq 3

echo "--- rm ---"
$BIN rm ETHERSCAN_API --force
test "$($BIN ls | wc -l)" -eq 2

echo "--- agent stop + auto-restart ---"
$BIN agent stop
sleep 0.2
test "$($BIN get RPC_URL)" = "https://rpc-v3.example.com"

echo "--- version ---"
contains "$($BIN --version)" "vars"

echo ""
echo "All smoke tests passed!"
