<?php

namespace App\Brain\Learning;

use App\Models\Lesson;

/**
 * Validator role: decides whether a quarantined Lesson is trustworthy enough
 * to be considered for promotion.
 *
 * The distinction that matters here is *how the claim was obtained*:
 *
 * - Filesystem observations ("this project exists at this path") can be
 *   re-checked against reality, so this really validates them — if the file
 *   has since been deleted or moved, the Lesson is rejected rather than
 *   promoted into permanent knowledge.
 * - Chat-derived Lessons are inferences made by a language model about what
 *   Petar meant. Nothing here can verify those, so they stay quarantined
 *   until a human approves them in the admin UI. Auto-promoting them would
 *   let a small local model quietly write its own guesses into long-term
 *   memory as fact.
 */
class Validator
{
    public function statusFor(Lesson $lesson): string
    {
        $path = $this->observedPath($lesson->source);

        if ($path === null) {
            return 'proposed'; // model inference — human gate
        }

        return file_exists($path) ? 'validated' : 'rejected';
    }

    public function isMachineVerifiable(Lesson $lesson): bool
    {
        return $this->observedPath($lesson->source) !== null;
    }

    private function observedPath(?string $source): ?string
    {
        if ($source === null) {
            return null;
        }

        foreach (['project:', 'document:'] as $prefix) {
            if (str_starts_with($source, $prefix)) {
                return substr($source, strlen($prefix));
            }
        }

        return null;
    }
}
