{
  description = "instant-bosh - A containerized BOSH director for local development and testing";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = {
          default = self.packages.${system}.ibosh;
          
          ibosh = pkgs.buildGoModule {
            pname = "ibosh";
            version = "0.1.0";
            
            src = ./.;
            
            vendorHash = "sha256-joiuLlTgl156ZhLWICjfJhSYK3LRuWhOaTDn+1kMTck=";
            
            subPackages = [ "cmd/ibosh" ];
            
            ldflags = [ "-s" "-w" ];
            
            # Install shell completions
            postInstall = ''
              installShellCompletion --cmd ibosh \
                --bash <($out/bin/ibosh --generate-bash-completion) \
                --zsh <($out/bin/ibosh --generate-zsh-completion) \
                --fish <($out/bin/ibosh --generate-fish-completion)
            '';
            
            nativeBuildInputs = [ pkgs.installShellFiles ];
            
            meta = with pkgs.lib; {
              description = "instant-bosh CLI - Manage containerized BOSH directors";
              homepage = "https://github.com/rkoster/instant-bosh";
              license = licenses.bsl11;
              maintainers = [ ];
              mainProgram = "ibosh";
            };
          };
        };

        # Development shell with dependencies
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
          ];
        };

        # Apps for easy running
        apps = {
          default = self.apps.${system}.ibosh;
          
          ibosh = {
            type = "app";
            program = "${self.packages.${system}.ibosh}/bin/ibosh";
          };
        };
      }
    );
}
