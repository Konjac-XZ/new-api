#!/usr/bin/env bash

set -Eeuo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
web_dir="$repo_dir/web"
default_web_dir="$repo_dir/web/default"
classic_web_dir="$repo_dir/web/classic"
binary="${BINARY:-$repo_dir/new-api}"

bun_bin="${BUN:-bun}"
install_deps="${INSTALL_DEPS:-auto}"
build_default=1
build_classic=0
skip_go_build=0
run_server=1
clean=0
app_args=()

usage() {
  cat <<'EOF'
Usage: ./run-local.sh [options] [-- new-api args...]

Build the embedded Default UI, compile the Go binary, and start new-api.

Options:
  --skip-web        Skip Default UI build.
  --classic         Also rebuild Classic UI instead of only checking its dist.
  --skip-go-build   Skip Go build and run the existing binary.
  --no-run          Build only; do not start the server.
  --install         Run bun install --frozen-lockfile before frontend builds.
  --clean           Remove Default UI dist and the target binary first.
  -h, --help        Show this help.

Environment:
  BINARY=/path/bin      Output binary path. Default: ./new-api
  BUN=/path/bun         Bun executable. Default: bun, then ~/.bun/bin/bun
  INSTALL_DEPS=0|1|auto Dependency install policy. Default: auto
  GOFLAGS='...'         Extra flags consumed by go build.
EOF
}

log() {
  printf '\n==> %s\n' "$*"
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

while (($#)); do
  case "$1" in
    --skip-web)
      build_default=0
      ;;
    --classic)
      build_classic=1
      ;;
    --skip-go-build)
      skip_go_build=1
      ;;
    --no-run)
      run_server=0
      ;;
    --install)
      install_deps=1
      ;;
    --clean)
      clean=1
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    --)
      shift
      app_args=("$@")
      break
      ;;
    *)
      app_args+=("$1")
      ;;
  esac
  shift
done

if ! command -v "$bun_bin" >/dev/null 2>&1; then
  if [[ -x "$HOME/.bun/bin/bun" ]]; then
    bun_bin="$HOME/.bun/bin/bun"
  fi
fi

require_command "$bun_bin"
require_command go

start_time=$SECONDS
version="$(<"$repo_dir/VERSION")"

if ((clean)); then
  log "Cleaning build outputs"
  rm -rf "$default_web_dir/dist"
  rm -f "$binary"
fi

if ((build_default || build_classic)); then
  if [[ "$install_deps" == "1" || ( "$install_deps" == "auto" && ! -d "$web_dir/node_modules" ) ]]; then
    log "Installing frontend dependencies"
    (cd "$web_dir" && "$bun_bin" install --frozen-lockfile)
  fi
fi

if ((build_default)); then
  log "Building Default UI"
  (
    cd "$default_web_dir"
    DISABLE_ESLINT_PLUGIN=true VITE_REACT_APP_VERSION="$version" "$bun_bin" run build
  )
fi

if ((build_classic)); then
  log "Building Classic UI"
  (
    cd "$classic_web_dir"
    VITE_REACT_APP_VERSION="$version" "$bun_bin" run build
  )
elif [[ ! -f "$classic_web_dir/dist/index.html" ]]; then
  die "web/classic/dist/index.html is missing; rerun with --classic so go:embed can compile"
fi

[[ -f "$default_web_dir/dist/index.html" ]] || die "web/default/dist/index.html is missing"

if ((!skip_go_build)); then
  log "Building Go binary"
  (cd "$repo_dir" && go build -o "$binary" .)
fi

if ((!run_server)); then
  log "Done in ${SECONDS}s"
  exit 0
fi

[[ -x "$binary" ]] || chmod +x "$binary"

log "Starting $binary"
printf 'Built in %ss\n' "$((SECONDS - start_time))"
cd "$repo_dir"
exec "$binary" "${app_args[@]}"
