<?php

namespace App\Brain\Learning;

use App\Brain\Memory\MemoryStore;
use App\Models\KnowledgeFact;
use App\Models\Lesson;

/**
 * Curator role: turns validated Lessons into durable knowledge, and refuses to
 * store the same fact twice.
 *
 * Deduplication is semantic rather than exact-match, because the same fact
 * genuinely arrives in different words — the scanners find several projects
 * mirrored between the external drive and ~/Projects, and a chat Lesson can
 * restate something already known.
 */
class Curator
{
    /**
     * Cosine similarity above which two facts are treated as the same thing.
     * Deliberately strict: wrongly merging two distinct facts loses knowledge,
     * while a near-duplicate slipping through is merely noise a human can tidy.
     */
    private const DUPLICATE_THRESHOLD = 0.95;

    public function __construct(private readonly MemoryStore $memory)
    {
    }

    public function promote(Lesson $lesson): ?KnowledgeFact
    {
        $closest = $this->memory->closestKnowledgeToLesson($lesson);

        if ($closest && $closest->similarity >= self::DUPLICATE_THRESHOLD) {
            $lesson->update(['status' => 'rejected']);

            return null;
        }

        $fact = KnowledgeFact::create([
            'promoted_from_lesson_id' => $lesson->id,
            'category' => $this->categoryFor($lesson),
            'content' => $lesson->content,
        ]);

        $this->memory->embedKnowledgeFact($fact);
        $lesson->update(['status' => 'promoted']);

        return $fact;
    }

    private function categoryFor(Lesson $lesson): string
    {
        return match (true) {
            str_starts_with((string) $lesson->source, 'project:') => 'project',
            str_starts_with((string) $lesson->source, 'document:') => 'document',
            default => 'conversation',
        };
    }
}
