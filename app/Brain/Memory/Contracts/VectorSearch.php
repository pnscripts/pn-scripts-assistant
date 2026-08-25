<?php

namespace App\Brain\Memory\Contracts;

/**
 * Similarity search over stored memories.
 *
 * This exists because vector search is the *only* part of Pnexus tied to
 * Postgres — everything else is storage-agnostic Eloquent. Keeping it behind an
 * interface is what allows the same codebase to run as a Docker service (pgvector)
 * and as a self-contained desktop app (SQLite). See ADR 0001.
 *
 * Similarity is cosine, normalised to 0..1 where 1 is identical, regardless of
 * how the underlying engine expresses distance.
 */
interface VectorSearch
{
    /** @param  array<int, float>  $vector */
    public function store(string $table, int $id, array $vector): void;

    /** @return array<int, float>|null */
    public function fetch(string $table, int $id): ?array;

    /**
     * Rows most similar to $vector, best first.
     *
     * @param  array<int, float>  $vector
     * @param  array<int, string>  $columns  columns to return alongside `similarity`
     * @return array<int, object>
     */
    public function search(string $table, array $vector, array $columns, int $limit, float $minSimilarity): array;
}
