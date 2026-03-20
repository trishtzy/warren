{
  description = "Rabbit Hole — dev environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    systems.url = "github:nix-systems/default";
    devenv.url = "github:cachix/devenv";
  };

  outputs = { self, nixpkgs, devenv, systems, ... } @ inputs:
    let
      forEachSystem = nixpkgs.lib.genAttrs (import systems);
    in
    {
      packages = forEachSystem
        (system:
          let
            pkgs = nixpkgs.legacyPackages.${system};
          in
          {
            default = pkgs.buildGoModule {
              pname = "warren";
              version = self.shortRev or self.dirtyShortRev or "dev";
              src = ./.;
              vendorHash = null;
              subPackages = [ "cmd/server" ];
              ldflags = [ "-s" "-w" ];
              env.CGO_ENABLED = "0";
            };
          });

      devShells = forEachSystem
        (system:
          let
            pkgs = nixpkgs.legacyPackages.${system};
          in
          {
            default = devenv.lib.mkShell {
              inherit inputs pkgs;
              modules = [
                {
                  languages.go = {
                    enable = true;
                  };

                  services.postgres = {
                    enable = true;
                    package = pkgs.postgresql_16;
                    listen_addresses = "127.0.0.1";
                    initialDatabases = [
                      { name = "rabbithole"; }
                      { name = "rabbithole_test"; }
                    ];
                    initialScript = ''
                      CREATE USER rabbithole WITH SUPERUSER PASSWORD 'rabbithole';
                      ALTER DATABASE rabbithole OWNER TO rabbithole;
                      ALTER DATABASE rabbithole_test OWNER TO rabbithole;
                    '';
                  };

                  packages = with pkgs; [
                    postgresql_16
                    sqlc
                    goose
                    gopls
                    golangci-lint
                  ];

                  git-hooks.hooks = {
                    gofmt = {
                      enable = true;
                      stages = [ "pre-push" ];
                    };
                    golangci-lint = {
                      enable = true;
                      stages = [ "pre-push" ];
                    };
                  };

                  scripts = {
                    build.exec = ''
                      go build -o bin/server ./cmd/server/
                    '';

                    dev.exec = ''
                      go run ./cmd/server/
                    '';

                    test.exec = ''
                      go test -v ./...
                    '';

                    lint.exec = ''
                      golangci-lint run
                    '';

                    fmt.exec = ''
                      go fmt ./...
                    '';

                    migrate-up.exec = ''
                      goose -dir migrations postgres "postgresql://rabbithole:rabbithole@127.0.0.1:5432/rabbithole?sslmode=disable" up
                    '';

                    migrate-down.exec = ''
                      goose -dir migrations postgres "postgresql://rabbithole:rabbithole@127.0.0.1:5432/rabbithole?sslmode=disable" down
                    '';

                    generate.exec = ''
                      sqlc generate
                    '';

                    clean.exec = ''
                      rm -rf bin/
                    '';
                  };

                  enterShell = ''
                    echo "rabbit-hole dev environment loaded"
                    echo "  go:    $(go version | cut -d' ' -f3)"
                    echo "  psql:  $(psql --version | head -1)"
                    echo "  sqlc:  $(sqlc version 2>&1)"
                    echo "  goose: $(goose --version 2>&1 | head -1)"
                    echo ""
                    echo "Commands: build, dev, test, lint, fmt, migrate-up, migrate-down, generate, clean"
                  '';
                }
              ];
            };
          });
    };
}
