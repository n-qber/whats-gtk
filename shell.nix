{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    go
    pkg-config
    gobject-introspection
  ];

  buildInputs = with pkgs; [
    # The Big Three for your project
    gtk4
    libadwaita
    alsa-lib

    # The supporting cast
    glib
    cairo
    graphene
    pango
    gdk-pixbuf
    
    # Ensures icons and themes load correctly during testing
    adwaita-icon-theme
  ];

  shellHook = ''
    export CGO_ENABLED=1
    echo "Modern GNOME (Adwaita) Go environment ready."
  '';
}
