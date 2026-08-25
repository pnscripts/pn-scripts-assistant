# Builds Pnexus as a single self-contained binary: the Laravel app, the PHP
# interpreter and a production web server, with no external dependencies. This
# is what turns "a web app that needs Docker" into "an app you can hand someone".
#
# Not intended to run on a developer laptop: compiling PHP statically needs
# roughly 10GB of scratch space. CI is the right place (see
# .github/workflows/release.yml), and release artifacts belong there anyway.
FROM dunglas/frankenphp:static-builder-gnu

WORKDIR /go/src/app/dist/app

COPY . .

# Never bake a developer's .env or git history into a distributed binary — it
# would ship real credentials and machine-specific paths to every user.
RUN rm -rf .git .env .env.example storage/logs/* \
    && rm -rf clients/cli/pnexus clients/desktop/pnexus-desktop

RUN composer install --no-dev --no-interaction --prefer-dist --optimize-autoloader

# Desktop mode runs on SQLite, so pdo_sqlite must be compiled in; there is no
# extension loading at runtime in a static build.
ENV PHP_EXTENSIONS="pdo_sqlite,sqlite3,mbstring,openssl,curl,zip,fileinfo,filter,session,tokenizer,dom,xml,pcntl,sodium"

WORKDIR /go/src/app
RUN EMBED=dist/app/ ./build-static.sh

# Result: dist/frankenphp-linux-x86_64
