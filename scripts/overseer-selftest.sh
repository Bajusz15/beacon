#!/usr/bin/env bash
#
# overseer-selftest.sh — on-host self-test for the Beacon Proxmox overseer.
#
# Run this ON a real Proxmox VE host. It exercises the `beacon proxmox` CLI against the
# host's live `pvesh` and asserts they agree — the one thing normal CI can't do (there's no
# Proxmox in GitHub Actions). Non-destructive by default; no cloud/account required.
#
# Typical loop while iterating on the code:
#   # on your dev machine:
#   GOOS=linux GOARCH=amd64 go build -o beacon ./cmd/beacon
#   scp beacon root@<host>:/usr/local/bin/beacon
#   scp scripts/overseer-selftest.sh root@<host>:/root/
#   # on the host:
#   bash overseer-selftest.sh
#
# Options:
#   BEACON_BIN=/path/to/beacon   Override the binary (default: /usr/local/bin/beacon)
#   --start                      Also test the single-instance guard by starting a master
#                                (only used when none is running; the started one is stopped).
#
# Exit: 0 = all passed, 1 = one or more failures, 2 = preflight/environment error.

set -uo pipefail

BEACON="${BEACON_BIN:-/usr/local/bin/beacon}"
ALLOW_START=0
[[ "${1:-}" == "--start" ]] && ALLOW_START=1

if [[ -t 1 ]]; then
  RD=$'\033[0;31m'; GN=$'\033[0;32m'; YW=$'\033[0;33m'; BL=$'\033[1;34m'; CL=$'\033[0m'
else
  RD=""; GN=""; YW=""; BL=""; CL=""
fi

PASS=0; FAIL=0; SKIP=0
ok()   { echo -e "  ${GN}PASS${CL}  $*"; PASS=$((PASS+1)); }
bad()  { echo -e "  ${RD}FAIL${CL}  $*"; FAIL=$((FAIL+1)); }
skip() { echo -e "  ${YW}SKIP${CL}  $*"; SKIP=$((SKIP+1)); }
head() { echo -e "\n${BL}== $* ==${CL}"; }

# ---------- preflight --------------------------------------------------------
head "preflight"
command -v pvesh >/dev/null 2>&1 || { echo "  not a Proxmox host (pvesh missing)"; exit 2; }
[[ -x "$BEACON" ]] || { echo "  beacon binary not found/executable at: $BEACON (set BEACON_BIN)"; exit 2; }
if ! pvesh get /version --output-format json >/dev/null 2>&1; then
  echo "  pvesh present but not usable (run as root on the PVE host)"; exit 2
fi
HAVE_JQ=1; command -v jq >/dev/null 2>&1 || HAVE_JQ=0
[[ $HAVE_JQ -eq 1 ]] || echo "  ${YW}note:${CL} jq not installed — structural checks will be skipped (apt-get install -y jq)"
echo "  beacon: $BEACON"
echo "  pvesh : $(command -v pvesh)"

PVESH_VMS="$(pvesh get /cluster/resources --type vm --output-format json 2>/dev/null)"

# ---------- 1. list runs and has a header -----------------------------------
head "1. proxmox list"
if list_out="$("$BEACON" proxmox list 2>&1)"; then
  if grep -q '^VMID' <<<"$list_out"; then
    ok "\`beacon proxmox list\` exits 0 and prints a header"
  elif grep -q 'No VMs or containers' <<<"$list_out"; then
    ok "\`beacon proxmox list\` exits 0 (host has no guests)"
  else
    bad "list exited 0 but output looks wrong:\n$list_out"
  fi
else
  bad "\`beacon proxmox list\` exited non-zero:\n$list_out"
fi

# ---------- 2. --json is valid ----------------------------------------------
head "2. proxmox list --json"
if [[ $HAVE_JQ -eq 1 ]]; then
  if "$BEACON" proxmox list --json 2>/dev/null | jq -e . >/dev/null 2>&1; then
    ok "\`--json\` emits valid JSON"
  else
    bad "\`--json\` is not valid JSON"
  fi
else
  skip "needs jq"
fi

# ---------- 3. guest set matches pvesh (the real contract) ------------------
head "3. inventory matches live pvesh"
if [[ $HAVE_JQ -eq 1 ]]; then
  b_ids="$("$BEACON" proxmox list --json 2>/dev/null | jq -S 'map(.vmid) | sort')"
  p_ids="$(jq -S 'map(.vmid) | sort' <<<"$PVESH_VMS")"
  if [[ "$b_ids" == "$p_ids" ]]; then
    ok "overseer VMIDs match pvesh ($(jq 'length' <<<"$PVESH_VMS") guests)"
  else
    bad "VMID mismatch\n    beacon: $b_ids\n    pvesh : $p_ids"
  fi
else
  skip "needs jq"
fi

# ---------- 4. status counts are correct ------------------------------------
head "4. status counts"
if [[ $HAVE_JQ -eq 1 ]]; then
  total="$(jq 'length' <<<"$PVESH_VMS")"
  running="$(jq '[.[] | select(.status=="running")] | length' <<<"$PVESH_VMS")"
  if ! st="$("$BEACON" proxmox status 2>&1)"; then
    bad "\`beacon proxmox status\` exited non-zero:\n$st"
  elif [[ "$st" =~ ([0-9]+)/([0-9]+) ]]; then
    n="${BASH_REMATCH[1]}"; m="${BASH_REMATCH[2]}"
    if [[ "$n" == "$running" && "$m" == "$total" ]]; then
      ok "status reports $n/$m (matches pvesh)"
    else
      bad "status \"$st\" -> $n/$m, expected $running/$total"
    fi
  else
    bad "could not parse \`beacon proxmox status\` output:\n$st"
  fi
else
  skip "needs jq"
fi

# ---------- 5. overseer alias == proxmox ------------------------------------
head "5. overseer alias"
a="$("$BEACON" overseer list 2>/dev/null)"
c="$("$BEACON" proxmox list 2>/dev/null)"
if [[ "$a" == "$c" ]]; then
  ok "\`beacon overseer list\` == \`beacon proxmox list\`"
else
  bad "alias output differs from canonical"
fi

# ---------- 6. single-instance guard ----------------------------------------
head "6. single-instance guard"
if pgrep -f 'beacon (master|start)' >/dev/null 2>&1; then
  out="$("$BEACON" start 2>&1)"
  if grep -qi 'already running' <<<"$out"; then
    ok "a master is running; second \`beacon start\` refused"
  else
    bad "guard did not refuse a second start:\n$out"
  fi
elif [[ $ALLOW_START -eq 1 ]]; then
  start_out="$("$BEACON" start 2>&1)"
  started_pid="$(grep -oE 'pid [0-9]+' <<<"$start_out" | grep -oE '[0-9]+' | head -1)"
  sleep 2
  second="$("$BEACON" start 2>&1)"
  if grep -qi 'already running' <<<"$second"; then
    ok "started a master (pid ${started_pid:-?}); second start refused"
  else
    bad "guard did not refuse after starting one:\n$second"
  fi
  [[ -n "${started_pid:-}" ]] && kill "$started_pid" 2>/dev/null && echo "    (stopped the master we started, pid $started_pid)"
else
  skip "no master running — re-run with --start to test the guard"
fi

# ---------- summary ----------------------------------------------------------
head "summary"
echo -e "  ${GN}${PASS} passed${CL}, ${RD}${FAIL} failed${CL}, ${YW}${SKIP} skipped${CL}"
[[ $FAIL -eq 0 ]] && exit 0 || exit 1
