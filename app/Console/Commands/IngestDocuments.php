<?php

namespace App\Console\Commands;

use App\Brain\Learning\DocumentScanner;
use App\Models\Lesson;
use Illuminate\Console\Attributes\Description;
use Illuminate\Console\Attributes\Signature;
use Illuminate\Console\Command;

#[Signature('brain:ingest-documents')]
#[Description('Read-only, metadata-only scan of SCAN_DOCUMENTS_PATH; proposes a Lesson per file found')]
class IngestDocuments extends Command
{
    public function handle(DocumentScanner $scanner): int
    {
        $root = config('brain.scan.documents');

        if (! $root || ! is_dir($root)) {
            $this->error('Documents scan root is not mounted — check SCAN_DOCUMENTS_PATH in .env.');

            return self::FAILURE;
        }

        $proposed = 0;
        $skipped = 0;

        foreach ($scanner->scan($root) as $file) {
            $source = 'document:'.$file['path'];

            if (Lesson::where('source', $source)->exists()) {
                $skipped++;

                continue;
            }

            $kb = number_format($file['size'] / 1024, 1);
            $content = "Petar has a .{$file['extension']} file named \"{$file['name']}\" ".
                "({$kb} KB) at {$file['path']}, last modified {$file['modified_at']}.";

            if ($file['excerpt']) {
                $content .= ' Excerpt: '.$file['excerpt'];
            }

            Lesson::create([
                'source' => $source,
                'content' => $content,
                'status' => 'proposed',
                'confidence' => 'high',
            ]);

            $proposed++;
        }

        $this->info("Proposed {$proposed} new document Lessons, skipped {$skipped} already known.");

        return self::SUCCESS;
    }
}
