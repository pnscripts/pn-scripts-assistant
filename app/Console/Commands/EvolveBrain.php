<?php

namespace App\Console\Commands;

use App\Brain\Learning\Curator;
use App\Brain\Learning\Validator;
use App\Brain\Memory\MemoryStore;
use App\Models\Lesson;
use Illuminate\Console\Attributes\Description;
use Illuminate\Console\Attributes\Signature;
use Illuminate\Console\Command;

/**
 * "Update Brain" — walks quarantined Lessons through Validator then Curator,
 * so that what the brain has merely *noticed* becomes what it actually *knows*.
 */
#[Signature('brain:evolve {--dry-run : Report what would happen without writing anything}')]
#[Description('Validate quarantined Lessons and promote the trustworthy ones into durable knowledge')]
class EvolveBrain extends Command
{
    public function handle(Validator $validator, Curator $curator, MemoryStore $memory): int
    {
        $dryRun = (bool) $this->option('dry-run');

        $validated = 0;
        $rejectedMissing = 0;
        $heldForReview = 0;

        $this->info('Validator: checking quarantined Lessons...');

        foreach (Lesson::where('status', 'proposed')->cursor() as $lesson) {
            $status = $validator->statusFor($lesson);

            match ($status) {
                'validated' => $validated++,
                'rejected' => $rejectedMissing++,
                default => $heldForReview++,
            };

            if (! $dryRun && $status !== 'proposed') {
                $lesson->update(['status' => $status]);
            }
        }

        $this->line("  validated: {$validated}   rejected (path gone): {$rejectedMissing}   held for human review: {$heldForReview}");

        if ($dryRun) {
            $this->comment('Dry run — nothing written. Re-run without --dry-run to apply.');

            return self::SUCCESS;
        }

        $this->info('Embedding validated Lessons...');
        $embedded = 0;

        foreach (Lesson::where('status', 'validated')->whereRaw('embedding IS NULL')->cursor() as $lesson) {
            $memory->embedLesson($lesson);
            $embedded++;

            if ($embedded % 25 === 0) {
                $this->line("  embedded {$embedded}...");
            }
        }

        $this->line("  embedded: {$embedded}");

        $this->info('Curator: promoting to durable knowledge...');
        $promoted = 0;
        $duplicates = 0;

        foreach (Lesson::where('status', 'validated')->cursor() as $lesson) {
            $curator->promote($lesson) ? $promoted++ : $duplicates++;
        }

        $this->line("  promoted: {$promoted}   skipped as duplicates: {$duplicates}");

        $this->newLine();
        $this->info("Brain updated. {$promoted} new facts in long-term memory.");

        if ($heldForReview > 0) {
            $this->comment("{$heldForReview} model-inferred Lessons are waiting for your approval at /admin/lessons.");
        }

        return self::SUCCESS;
    }
}
