<?php

namespace App\Support\Schema;

use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

/**
 * Adds an embedding column in whatever form the current database understands.
 *
 * Postgres gets a real `vector` type backed by pgvector; SQLite gets JSON text,
 * because it has no vector type and the desktop build must install with no
 * extensions. Keeping the difference here means migrations stay readable and the
 * two deployment targets can't drift apart. See ADR 0001.
 */
class VectorColumn
{
    public static function add(string $table, string $column = 'embedding'): void
    {
        $dimensions = config('llm.embedding.dimensions');

        if (DB::getDriverName() === 'pgsql') {
            DB::statement("ALTER TABLE {$table} ADD COLUMN {$column} vector({$dimensions})");

            return;
        }

        Schema::table($table, fn (Blueprint $t) => $t->text($column)->nullable());
    }

    /** pgvector ships as an extension; other drivers need nothing. */
    public static function enableExtension(): void
    {
        if (DB::getDriverName() === 'pgsql') {
            DB::statement('CREATE EXTENSION IF NOT EXISTS vector');
        }
    }
}
