# Sourced (hidden) by the VHS tape. It builds gh-merge with -tags demo, which
# swaps GitHub for an in-memory fake (demo.go), so nothing real is merged.
# Run from the repo root:  vhs docs/tapes/demo.tape
GHM_DEMO="$(mktemp -d)/gh-merge"
go build -tags demo -o "$GHM_DEMO" . || return
gh() { if [ "$1" = merge ]; then shift; "$GHM_DEMO" "$@"; else command gh "$@"; fi; }
cd "$(mktemp -d)" || return
PROMPT='%F{blue}~/webapp%f %# '
clear
