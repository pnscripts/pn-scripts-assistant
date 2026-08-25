<?php

namespace App\Providers;

use App\Brain\Memory\Contracts\VectorSearch;
use App\Brain\Memory\Drivers\PgVectorSearch;
use App\Brain\Memory\Drivers\SqliteVectorSearch;
use Illuminate\Support\ServiceProvider;
use RuntimeException;

class AppServiceProvider extends ServiceProvider
{
    /**
     * Register any application services.
     */
    public function register(): void
    {
        // Chosen from the active connection rather than a separate setting, so
        // there is no way to run the SQLite driver against Postgres or vice
        // versa by misconfiguring one of two knobs. See ADR 0001.
        $this->app->singleton(VectorSearch::class, fn () => match ($driver = config('database.default')) {
            'pgsql' => new PgVectorSearch,
            'sqlite' => new SqliteVectorSearch,
            default => throw new RuntimeException(
                "No vector search driver for database connection [{$driver}]."
            ),
        });
    }

    /**
     * Bootstrap any application services.
     */
    public function boot(): void
    {
        //
    }
}
