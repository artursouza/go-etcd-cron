function exit_error {
  echo '>> Please run:'
  echo '>> $ nix run .#update'
  exit 1
}

function protos {
  TMPDIR=$(mktemp -d)
  trap 'rm -rf -- "$TMPDIR"' EXIT

  protoc -I $src \
    --go_out="$TMPDIR" \
    --go_opt=module=github.com/diagridio/go-etcd-cron \
    --go-grpc_opt=module=github.com/diagridio/go-etcd-cron \
    --go-grpc_out="$TMPDIR" \
    $src/proto/v1alpha1/*.proto

  if ! diff -q -r "$TMPDIR/grpc" "$src/grpc"; then
    echo '>> Proto files are not up to date.'
    exit_error
  fi

  echo '>> Proto files are up to date.'
}

function gomodules {
  TMPDIR=$(mktemp -d)
  trap 'rm -rf -- "$TMPDIR"' EXIT

  echo ">> Running 'go mod tidy' in '$src'..."
  cp --no-preserve=mode -r "$src"/* "$TMPDIR"
  ( cd "$TMPDIR" && go mod tidy -v )

  echo ">> Checking if Go modules are up to date..."
  if ! diff -q "$src/go.mod" "$TMPDIR/go.mod" || ! diff -q "$src/go.sum" "$TMPDIR/go.sum"; then
    echo '>> Go modules are not up to date'
    exit_error
  fi

  echo ">> Checking if 'gomod2nix.toml' is up to date..."
  gomod2nix --dir "$src" --outdir "$TMPDIR"
  if ! diff -q "$TMPDIR/gomod2nix.toml" "$src/gomod2nix.toml"; then
    echo '>> gomod2nix.toml is not up to date.'
    exit_error
  fi

  printf ">> '%s/gomod2nix.toml' is up to date\n" "$src"
}

function gotest {
  cd "$(git rev-parse --show-toplevel)"

  gofmt -s -l -e .
  go vet -v ./...
  #go test --race -v "$src/..."
  echo ">> Skipping tests as they currently hang."
  exit 1
}

echo ">> Checking protos..."
protos

echo ">> Checking Go modules..."
gomodules

echo ">> Checking Go..."
gotest
