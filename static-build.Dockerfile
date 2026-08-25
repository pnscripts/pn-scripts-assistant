# Builds Pnexus as a single self-contained binary: the Laravel app, the PHP
# interpreter and a production web server, with no external dependencies. This
# is what turns "a web app that needs Docker" into "an app you can hand someone".
#
# Not intended to run on a developer laptop: compiling PHP statically needs
# roughly 10GB of scratch space. CI is the right place (see
# .github/workflows/release.yml), and release artifacts belong there anyway.

# The static-builder image has no Composer, so dependencies are resolved here.
# The whole app is copied in rather than just composer.json, because an
# optimised autoloader is a classmap of real files and needs them present.
FROM composer:2 AS vendor

WORKDIR /app
COPY . .

# Never bake a developer's environment or git history into a distributed
# binary: it would ship real credentials and machine-specific paths to users.
RUN rm -rf .git .env storage/logs/* \
    && rm -f clients/cli/pnexus clients/desktop/pnexus-desktop

# Scripts are skipped because Laravel's post-install hooks run artisan, which
# would try to boot the app against a database that doesn't exist at build time.
RUN composer install \
      --no-dev \
      --no-interaction \
      --no-scripts \
      --prefer-dist \
      --optimize-autoloader \
      --ignore-platform-reqs

# Belt and braces alongside .dockerignore: any cached package manifest that
# survives here would list dev-only providers and kill the binary at boot.
RUN rm -f bootstrap/cache/*.php

FROM dunglas/frankenphp:static-builder-gnu

WORKDIR /go/src/app/dist/app
COPY --from=vendor /app .

# PHP_EXTENSIONS is deliberately left unset: build-static.sh then runs
# `spc dump-extensions` over composer.json/lock and derives exactly what the
# installed packages declare. A hand-written list rots — the first attempt here
# omitted ext-intl, which filament/support requires, and would have failed on it
# next. Desktop mode's SQLite requirement is declared in composer.json instead,
# where it is true regardless of how the app is built.
#
# libzip pulls in bzip2, xz and zstd symbols, but the builder's default library
# set excludes all three, so linking fails with undefined references. ext-zip
# arrives via openspout (Filament's spreadsheet export), so the libraries have to
# be added rather than the extension dropped. The defaults are repeated here
# because setting this variable replaces them wholesale.
ENV PHP_EXTENSION_LIBS="libavif,nghttp2,nghttp3,ngtcp2,watcher,bzip2,xz,zstd"

WORKDIR /go/src/app

# static-php-cli resolves release URLs for zlib, openssl, icu and friends through
# the GitHub API, which allows 60 unauthenticated calls per hour per IP. A couple
# of rebuilds in an afternoon exhausts that and the build dies mid-download with
# a 403 — an environmental failure that looks alarmingly like a code one.
#
# Mounted as a secret rather than passed as an ARG: build args are visible in
# `docker history`, and a token embedded in a distributable image is a token
# published to whoever downloads it.
RUN --mount=type=secret,id=github_token,required=false \
    GITHUB_TOKEN="$(cat /run/secrets/github_token 2>/dev/null || true)" \
    EMBED=dist/app/ ./build-static.sh

# Result: dist/frankenphp-linux-x86_64
