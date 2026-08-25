<?php

use Illuminate\Foundation\Application;
use Illuminate\Foundation\Configuration\Exceptions;
use Illuminate\Foundation\Configuration\Middleware;
use Illuminate\Http\Request;

$app = Application::configure(basePath: dirname(__DIR__))
    ->withRouting(
        web: __DIR__.'/../routes/web.php',
        api: __DIR__.'/../routes/api.php',
        commands: __DIR__.'/../routes/console.php',
        health: '/up',
    )
    ->withMiddleware(function (Middleware $middleware): void {
        //
    })
    ->withExceptions(function (Exceptions $exceptions): void {
        $exceptions->shouldRenderJsonWhen(
            fn (Request $request) => $request->is('api/*') || $request->expectsJson(),
        );
    })->create();

/*
 * In a standalone build the application lives inside the executable, so its
 * own storage/ directory cannot be written to — Laravel would fail on the
 * first compiled Blade view. Storage therefore moves beside the binary, which
 * is also where the SQLite database and .env already live, keeping the whole
 * brain in one copyable folder.
 *
 * getenv() rather than env(): this runs before the framework has loaded .env.
 * In a normal Docker or source checkout nothing is set and the default path
 * applies, so this is inert outside standalone builds.
 */
if ($storagePath = getenv('PN_BRAIN_STORAGE_PATH')) {
    $app->useStoragePath($storagePath);
}

/*
 * A standalone build unpacks itself into a temporary directory, so Laravel's
 * default of "read .env from the application root" means reading a file inside
 * the executable — which is not where anyone put their configuration. Without
 * this the app silently starts with defaults: it migrates into a throwaway
 * database and reports no encryption key, both of which look like unrelated
 * bugs.
 *
 * Environment variables alone are not a workaround. FrankenPHP serves requests
 * from several threads and does not reliably surface the parent process's
 * environment to all of them, so configuration has to come from a file the app
 * can actually find.
 *
 * PN_BRAIN_ROOT is set by the launcher to the directory holding the binary; that
 * folder holds .env, the SQLite database and storage, so the whole brain stays
 * one copyable directory.
 */
if ($root = getenv('PN_BRAIN_ROOT')) {
    $app->useEnvironmentPath($root);
}

return $app;
