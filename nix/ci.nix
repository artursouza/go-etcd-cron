{
src,
pkgs,
}:

let
  program = { name }: let
    sh = pkgs.writeShellApplication {
      inherit name;
      runtimeEnv = { "src" = src; };
      text = builtins.readFile ./sh/${name}.sh;
      runtimeInputs = with pkgs; [
        git
        go
        gomod2nix
        protobuf
        protoc-gen-go
        protoc-gen-go-grpc
        diffutils
      ];
    };
  in {type = "app"; program = "${sh}/bin/${name}";};

  check = program { name = "check"; };
  update = program { name = "update"; };

in {
  apps = {
    update = update;
    test = check;
    check = check;
    default = check;
  };
}
