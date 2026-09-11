{ pkgs ? import <nixpkgs> {} }:

let
  gdk-pixbuf-loaders = pkgs.symlinkJoin {
    name = "gdk-pixbuf-loaders";
    paths = [
      pkgs.gdk-pixbuf
      pkgs.webp-pixbuf-loader
      pkgs.librsvg
    ];
    postBuild = ''
      rm -f $out/lib/gdk-pixbuf-2.0/2.10.0/loaders.cache
      ${pkgs.gdk-pixbuf.dev}/bin/gdk-pixbuf-query-loaders $out/lib/gdk-pixbuf-2.0/2.10.0/loaders/*.so > $out/lib/gdk-pixbuf-2.0/2.10.0/loaders.cache
    '';
  };
  glib-schemas = pkgs.symlinkJoin {
    name = "glib-schemas";
    paths = [
      pkgs.gsettings-desktop-schemas
      pkgs.gtk4
      pkgs.libadwaita
    ];
    postBuild = ''
      mkdir -p $out/share/glib-2.0/schemas
      for d in ${pkgs.gsettings-desktop-schemas}/share/gsettings-schemas/* \
               ${pkgs.gtk4}/share/gsettings-schemas/* \
               ${pkgs.libadwaita}/share/gsettings-schemas/*; do
        if [ -d "$d/glib-2.0/schemas" ]; then
          cp -rn --dereference "$d/glib-2.0/schemas/"*.xml $out/share/glib-2.0/schemas/ 2>/dev/null || true
        fi
      done
      ${pkgs.glib.dev}/bin/glib-compile-schemas $out/share/glib-2.0/schemas/
    '';
  };
in
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    go
    gnumake
    pkg-config
    gobject-introspection
    sqlite
    mold
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
    webp-pixbuf-loader
    librsvg
    
    # Ensures icons and themes load correctly during testing
    adwaita-icon-theme
    gsettings-desktop-schemas
    glib-schemas
  ];

  shellHook = ''
    export CGO_ENABLED=1
    export CGO_CFLAGS="-O1"
    export CGO_CXXFLAGS="-O1"
    export CGO_LDFLAGS="-fuse-ld=mold"
    export GDK_PIXBUF_MODULE_FILE="${gdk-pixbuf-loaders}/lib/gdk-pixbuf-2.0/2.10.0/loaders.cache"
    export XDG_DATA_DIRS="${glib-schemas}/share:${pkgs.adwaita-icon-theme}/share:${pkgs.libadwaita}/share:${pkgs.gtk4}/share:$XDG_DATA_DIRS"
    echo "Modern GNOME (Adwaita) Go environment ready with mold fast linker and WebP sticker support."
  '';
}

