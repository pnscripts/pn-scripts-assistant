<?php

namespace App\Brain\Memory\Drivers;

use App\Brain\Memory\Contracts\VectorSearch;
use Illuminate\Support\Facades\DB;

/**
 * Server mode. Postgres does the work via pgvector's `<=>` cosine-distance
 * operator, which can use an index and never loads vectors into PHP.
 */
class PgVectorSearch implements VectorSearch
{
    public function store(string $table, int $id, array $vector): void
    {
        DB::update("UPDATE {$table} SET embedding = ?::vector WHERE id = ?", [
            $this->toLiteral($vector), $id,
        ]);
    }

    public function fetch(string $table, int $id): ?array
    {
        $row = DB::selectOne("SELECT embedding::text AS embedding FROM {$table} WHERE id = ?", [$id]);

        if (! $row || $row->embedding === null) {
            return null;
        }

        return array_map('floatval', explode(',', trim($row->embedding, '[]')));
    }

    public function search(string $table, array $vector, array $columns, int $limit, float $minSimilarity): array
    {
        $literal = $this->toLiteral($vector);
        $select = implode(', ', $columns);

        // `<=>` is cosine distance (0 identical, 2 opposite), so similarity is 1 - distance.
        return DB::select(
            "SELECT {$select}, 1 - (embedding <=> ?::vector) AS similarity
             FROM {$table}
             WHERE embedding IS NOT NULL
               AND 1 - (embedding <=> ?::vector) >= ?
             ORDER BY embedding <=> ?::vector
             LIMIT ?",
            [$literal, $literal, $minSimilarity, $literal, $limit]
        );
    }

    private function toLiteral(array $vector): string
    {
        return '['.implode(',', array_map(fn ($v) => (float) $v, $vector)).']';
    }
}
