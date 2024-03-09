{
pkgs,
}:

let
  checkgomod2nixSH = pkgs.writeShellApplication {
    name = "check-gomod2nix";
    runtimeInputs = with pkgs; [ gomod2nix ];
    text = ''
      TMPDIR=$(mktemp -d)
      trap 'rm -rf -- "$TMPDIR"' EXIT
      gomod2nix --dir "$1" --outdir "$TMPDIR"
      if ! diff -q "$TMPDIR/gomod2nix.toml" "$1/gomod2nix.toml"; then
        echo '>> gomod2nix.toml is not up to date. Please run:'
        echo '>> $ nix run .#update'
        exit 1
      fi
      echo ">> \"$1/gomod2nix.toml\" is up to date"
    '';
  };

  checkProtoSH = pkgs.writeShellApplication {
    name = "check-protos";
    runtimeInputs = with pkgs; [
      protobuf
      protoc-gen-go
      protoc-gen-go-grpc
      diffutils
    ];
    text = ''
      TMPDIR=$(mktemp -d)
      trap 'rm -rf -- "$TMPDIR"' EXIT
      protoc --go_out="$TMPDIR" \
        --go_opt=module=github.com/diagridio/go-etcd-cron \
        --go-grpc_opt=module=github.com/diagridio/go-etcd-cron \
        --go-grpc_out="$TMPDIR" \
        ./proto/v1alpha1/*.proto
      if ! diff -q -r "$TMPDIR/grpc" ./grpc; then
        echo '>> proto files are not up to date. Please run:'
        echo '>> $ nix run .#update'
        exit 1
      fi
    '';
  };

  updateSH = pkgs.writeShellApplication {
    name = "update";
    runtimeInputs = with pkgs; [
      git
      gomod2nix
      protobuf
      protoc-gen-go
      protoc-gen-go-grpc
    ];
    text = ''
      cd "$(git rev-parse --show-toplevel)"
      gomod2nix
      find grpc
      rm -rf ./grpc && mkdir -p grpc
      find grpc
      protoc --go_out=. \
        --go_opt=module=github.com/diagridio/go-etcd-cron \
        --go-grpc_opt=module=github.com/diagridio/go-etcd-cron \
        --go-grpc_out=. \
        ./proto/v1alpha1/*.proto
      echo '>> Updated. Please commit the changes.'
    '';
  };

  testSH = pkgs.writeShellApplication {
    name = "test";
    runtimeInputs = with pkgs; [
      git
      go
      checkgomod2nixSH
      checkProtoSH
    ];
    text = ''
      cd "$(git rev-parse --show-toplevel)"
      check-gomod2nix .
      check-protos
      gofmt -s -l -e .
      go vet -v ./...
      #go test --race -v ./...
      echo ">> Skipping tests as they currently hang."
      exit 1
    '';
  };
in {
  apps = rec {
    update = {type = "app"; program = "${updateSH}/bin/update";};
    test = {type = "app"; program = "${testSH}/bin/test";};
    default = test;
  };
}
