cd "$(git rev-parse --show-toplevel)"

echo '>> Updating proto generation...'
rm -rf ./grpc && mkdir -p grpc
protoc --go_out=. \
  --go_opt=module=github.com/diagridio/go-etcd-cron \
  --go-grpc_opt=module=github.com/diagridio/go-etcd-cron \
  --go-grpc_out=. \
  ./proto/v1alpha1/*.proto

echo '>> Updating Go modules...'
go mod tidy -v

echo '>> Updating 'gomod2nix.toml'...'
gomod2nix

echo '>> Updated. Please commit the changes.'
