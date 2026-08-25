<?php

namespace App\Brain\Memory;

use App\Brain\Llm\EmbeddingService;
use App\Models\KnowledgeFact;
use App\Models\Lesson;
use Illuminate\Support\Collection;
use Illuminate\Support\Facades\DB;

/**
 * Reading and writing the brain's long-term memory.
 *
 * Vector maths happens in Postgres (pgvector), not here — `<=>` is cosine
 * distance, where 0 means identical and 2 means opposite, so similarity is
 * 1 - distance.
 */
class MemoryStore
{
    public function __construct(private readonly EmbeddingService $embeddings)
    {
    }

    public function embedLesson(Lesson $lesson): void
    {
        $literal = $this->embeddings->toVectorLiteral($this->embeddings->embed($lesson->content));

        DB::update('UPDATE lessons SET embedding = ?::vector WHERE id = ?', [$literal, $lesson->id]);
    }

    public function embedKnowledgeFact(KnowledgeFact $fact): void
    {
        $literal = $this->embeddings->toVectorLiteral($this->embeddings->embed($fact->content));

        DB::update('UPDATE knowledge_facts SET embedding = ?::vector WHERE id = ?', [$literal, $fact->id]);
    }

    /**
     * Durable knowledge most relevant to some text, best match first.
     *
     * @return Collection<int, object{id: int, content: string, category: ?string, similarity: float}>
     */
    public function recall(string $query, int $limit = 12, float $minSimilarity = 0.5): Collection
    {
        $literal = $this->embeddings->toVectorLiteral($this->embeddings->embed($query));

        return collect(DB::select(
            'SELECT id, content, category, 1 - (embedding <=> ?::vector) AS similarity
             FROM knowledge_facts
             WHERE embedding IS NOT NULL
               AND 1 - (embedding <=> ?::vector) >= ?
             ORDER BY embedding <=> ?::vector
             LIMIT ?',
            [$literal, $literal, $minSimilarity, $literal, $limit]
        ));
    }

    /**
     * The closest existing knowledge to a Lesson, used to avoid promoting the
     * same fact twice in slightly different words.
     */
    public function closestKnowledgeToLesson(Lesson $lesson): ?object
    {
        $rows = DB::select(
            'SELECT k.id, k.content, 1 - (k.embedding <=> l.embedding) AS similarity
             FROM knowledge_facts k, lessons l
             WHERE l.id = ? AND l.embedding IS NOT NULL AND k.embedding IS NOT NULL
             ORDER BY k.embedding <=> l.embedding
             LIMIT 1',
            [$lesson->id]
        );

        return $rows[0] ?? null;
    }
}
