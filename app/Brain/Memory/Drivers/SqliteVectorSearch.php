<?php

namespace App\Brain\Memory\Drivers;

use App\Brain\Memory\Contracts\VectorSearch;
use Illuminate\Support\Facades\DB;

/**
 * Desktop mode. SQLite has no vector type, so vectors are stored as JSON and
 * cosine similarity is computed here.
 *
 * This is a linear scan, and that is a deliberate trade rather than an oversight:
 * it removes the last dependency standing between this app and a single-file
 * install (ADR 0001), and at personal scale the cost is irrelevant — a few
 * thousand memories compare in well under a tenth of a second.
 *
 * It degrades predictably, though. Somewhere in the tens of thousands of
 * memories this becomes the slowest part of a reply, and the answer then is a
 * real index (sqlite-vec) or server mode — not a bigger loop.
 */
class SqliteVectorSearch implements VectorSearch
{
    public function store(string $table, int $id, array $vector): void
    {
        DB::update("UPDATE {$table} SET embedding = ? WHERE id = ?", [json_encode($vector), $id]);
    }

    public function fetch(string $table, int $id): ?array
    {
        $row = DB::selectOne("SELECT embedding FROM {$table} WHERE id = ?", [$id]);

        return $row?->embedding ? json_decode($row->embedding, true) : null;
    }

    public function search(string $table, array $vector, array $columns, int $limit, float $minSimilarity): array
    {
        $select = implode(', ', array_merge($columns, ['embedding']));
        $rows = DB::select("SELECT {$select} FROM {$table} WHERE embedding IS NOT NULL");

        $queryNorm = $this->norm($vector);

        if ($queryNorm === 0.0) {
            return [];
        }

        $scored = [];

        foreach ($rows as $row) {
            $stored = json_decode($row->embedding, true);

            if (! is_array($stored) || count($stored) !== count($vector)) {
                continue; // dimension mismatch: a leftover from a different embedding model
            }

            $similarity = $this->cosine($vector, $stored, $queryNorm);

            if ($similarity >= $minSimilarity) {
                unset($row->embedding); // callers want the columns they asked for, not the raw vector
                $row->similarity = $similarity;
                $scored[] = $row;
            }
        }

        usort($scored, fn ($a, $b) => $b->similarity <=> $a->similarity);

        return array_slice($scored, 0, $limit);
    }

    private function cosine(array $a, array $b, float $normA): float
    {
        $dot = 0.0;
        $sumB = 0.0;

        foreach ($a as $i => $valueA) {
            $valueB = $b[$i];
            $dot += $valueA * $valueB;
            $sumB += $valueB * $valueB;
        }

        $normB = sqrt($sumB);

        return $normB === 0.0 ? 0.0 : $dot / ($normA * $normB);
    }

    private function norm(array $vector): float
    {
        return sqrt(array_sum(array_map(fn ($v) => $v * $v, $vector)));
    }
}
