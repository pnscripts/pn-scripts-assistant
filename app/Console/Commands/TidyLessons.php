<?php

namespace App\Console\Commands;

use App\Brain\Learning\ExtractLessonJob;
use App\Brain\Persona;
use App\Models\Lesson;
use Illuminate\Console\Attributes\Description;
use Illuminate\Console\Attributes\Signature;
use Illuminate\Console\Command;
use ReflectionMethod;

/**
 * Clears self-description out of the review queue.
 *
 * Extraction now refuses to record the assistant describing itself, but lessons
 * captured before that fix are still waiting for a human. Reviewing a queue that
 * is mostly "I am Sage" and "I am Vesper" teaches someone to skim, which is
 * exactly the habit the review step cannot afford.
 *
 * Re-runnable: as the guard learns new shapes of noise, this reapplies it to
 * whatever is still pending.
 */
#[Signature('brain:tidy-lessons {--dry-run : Show what would be rejected without changing anything}')]
#[Description('Reject queued lessons that describe the assistant rather than its owner')]
class TidyLessons extends Command
{
    public function handle(): int
    {
        $dryRun = (bool) $this->option('dry-run');
        $guard = new ReflectionMethod(ExtractLessonJob::class, 'isAboutTheAssistant');
        $owner = Persona::owner();

        $rejected = 0;
        $kept = [];

        foreach (Lesson::where('status', 'proposed')->orderBy('id')->get() as $lesson) {
            if ($guard->invoke(null, $lesson->content, $owner)) {
                $this->line("  <fg=gray>reject</> [{$lesson->id}] ".str($lesson->content)->limit(72));

                if (! $dryRun) {
                    $lesson->update(['status' => 'rejected']);
                }

                $rejected++;

                continue;
            }

            $kept[] = $lesson;
        }

        $this->newLine();
        $this->info(($dryRun ? 'Would reject ' : 'Rejected ')."{$rejected} self-descriptions.");

        if ($kept === []) {
            $this->line('  Nothing left needing your judgement.');

            return self::SUCCESS;
        }

        $this->newLine();
        $this->comment('Still awaiting your review — these are claims about you, so only you can confirm them:');

        foreach ($kept as $lesson) {
            $this->line("  <fg=yellow>[{$lesson->id}]</> ".str($lesson->content)->limit(90));
        }

        $this->newLine();
        $this->line('  Approve or reject at /admin/lessons');

        return self::SUCCESS;
    }
}
