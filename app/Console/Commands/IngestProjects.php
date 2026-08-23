<?php

namespace App\Console\Commands;

use App\Brain\Learning\ProjectScanner;
use App\Models\Lesson;
use Illuminate\Console\Attributes\Description;
use Illuminate\Console\Attributes\Signature;
use Illuminate\Console\Command;

#[Signature('brain:ingest-projects')]
#[Description('Read-only scan of configured project directories; proposes a Lesson per project found')]
class IngestProjects extends Command
{
    public function handle(ProjectScanner $scanner): int
    {
        $roots = array_filter([
            config('brain.scan.dev_projects'),
            config('brain.scan.home_projects'),
        ], fn ($p) => $p && is_dir($p));

        if (empty($roots)) {
            $this->error('No scan roots are mounted — check SCAN_DEV_PROJECTS_PATH / SCAN_HOME_PROJECTS_PATH in .env.');

            return self::FAILURE;
        }

        $proposed = 0;
        $skipped = 0;

        foreach ($roots as $root) {
            foreach ($scanner->scan($root) as $project) {
                $source = 'project:'.$project['path'];

                if (Lesson::where('source', $source)->exists()) {
                    $skipped++;

                    continue;
                }

                $content = "Petar has a {$project['stack']} project called \"{$project['name']}\" ".
                    "at {$project['path']}, last modified {$project['modified_at']}.";

                if ($project['readme_excerpt']) {
                    $content .= ' README excerpt: '.$project['readme_excerpt'];
                }

                Lesson::create([
                    'source' => $source,
                    'content' => $content,
                    'status' => 'proposed',
                    // Observed directly from the filesystem, not inferred by a model —
                    // still quarantined like any other Lesson, just a stronger prior.
                    'confidence' => 'high',
                ]);

                $proposed++;
            }
        }

        $this->info("Proposed {$proposed} new project Lessons, skipped {$skipped} already known.");

        return self::SUCCESS;
    }
}
