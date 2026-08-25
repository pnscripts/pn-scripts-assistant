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

FROM dunglas/frankenphp:static-builder-gnu

WORKDIR /go/src/app/dist/app
COPY --from=vendor /app .

# Desktop mode runs on SQLite, so pdo_sqlite must be compiled in — a static
# build has no runtime extension loading.
ENV PHP_EXTENSIONS="pdo_sqlite,sqlite3,mbstring,openssl,curl,zip,fileinfo,filter,session,tokenizer,dom,xml,pcntl,sodium"

WORKDIR /go/src/app
RUN EMBED=dist/app/ ./build-static.sh

# Result: dist/frankenphp-linux-x86_64
