<?php

namespace App\Brain\Memory;

use App\Brain\Llm\EmbeddingService;
use App\Brain\Memory\Contracts\VectorSearch;
use App\Models\KnowledgeFact;
use App\Models\Lesson;
use Illuminate\Support\Collection;

/**
 * Reading and writing the brain's long-term memory.
 *
 * Deliberately knows nothing about *how* similarity is computed — that sits
 * behind VectorSearch so the same code runs on pgvector in server mode and on
 * SQLite in a self-contained desktop build (ADR 0001).
 */
class MemoryStore
{
    public function __construct(
        private readonly EmbeddingService $embeddings,
        private readonly VectorSearch $vectors,
    ) {
    }

    public function embedLesson(Lesson $lesson): void
    {
        $this->vectors->store('lessons', $lesson->id, $this->embeddings->embed($lesson->content));
    }

    public function embedKnowledgeFact(KnowledgeFact $fact): void
    {
        $this->vectors->store('knowledge_facts', $fact->id, $this->embeddings->embed($fact->content));
    }

    /**
     * Durable knowledge most relevant to some text, best match first.
     *
     * @return Collection<int, object>
     */
    public function recall(string $query, int $limit = 12, float $minSimilarity = 0.5): Collection
    {
        return collect($this->vectors->search(
            table: 'knowledge_facts',
            vector: $this->embeddings->embed($query),
            columns: ['id', 'content', 'category'],
            limit: $limit,
            minSimilarity: $minSimilarity,
        ));
    }

    /**
     * The closest existing knowledge to a Lesson, used to avoid promoting the
     * same fact twice in slightly different words.
     *
     * Reuses the Lesson's stored vector rather than re-embedding its text: the
     * Lesson was embedded moments earlier by the same pipeline, so a second call
     * would cost an inference for an identical result.
     */
    public function closestKnowledgeToLesson(Lesson $lesson): ?object
    {
        $vector = $this->vectors->fetch('lessons', $lesson->id);

        if ($vector === null) {
            return null;
        }

        $matches = $this->vectors->search(
            table: 'knowledge_facts',
            vector: $vector,
            columns: ['id', 'content'],
            limit: 1,
            // No floor: the caller compares against its own duplicate threshold,
            // and filtering here would hide a near-miss it may care about.
            minSimilarity: -1.0,
        );

        return $matches[0] ?? null;
    }
}
