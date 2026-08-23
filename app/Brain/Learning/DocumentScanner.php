<?php

namespace App\Brain\Learning;

/**
 * Read-only filesystem scan (see compose.yaml :ro mounts — this never writes
 * anything). Deliberately metadata-only: file name, type, size, date. Content
 * excerpts are only taken from plain-text/markdown files — never .docx/.odt/
 * .pdf/.xlsx/etc, since those are exactly the types most likely to hold IDs,
 * financial records, or contracts. Extend this per-file/per-folder later if
 * genuinely needed, rather than defaulting to reading everything.
 */
class DocumentScanner
{
    private const TEXT_EXTENSIONS = ['md', 'txt'];

    private const SKIP_DIRS = ['.git', '.cache', 'node_modules'];

    private const MAX_DEPTH = 8;

    private const MAX_EXCERPT_SOURCE_BYTES = 50_000;

    /** @return array<int, array{path: string, name: string, extension: string, size: int, modified_at: string, excerpt: ?string}> */
    public function scan(string $root): array
    {
        $files = [];
        $this->walk($root, 0, $files);

        return $files;
    }

    private function walk(string $dir, int $depth, array &$files): void
    {
        if ($depth > self::MAX_DEPTH || ! is_dir($dir) || ! is_readable($dir)) {
            return;
        }

        $entries = @scandir($dir) ?: [];
        foreach ($entries as $entry) {
            if ($entry === '.' || $entry === '..' || in_array($entry, self::SKIP_DIRS, true)) {
                continue;
            }

            $path = "$dir/$entry";

            if (is_link($path)) {
                continue; // avoid symlink loops (this drive has a self-referential one)
            }

            if (is_dir($path)) {
                $this->walk($path, $depth + 1, $files);

                continue;
            }

            if (is_file($path) && is_readable($path)) {
                $files[] = $this->describe($path);
            }
        }
    }

    private function describe(string $path): array
    {
        $extension = strtolower(pathinfo($path, PATHINFO_EXTENSION));
        $size = filesize($path) ?: 0;
        $excerpt = null;

        if (in_array($extension, self::TEXT_EXTENSIONS, true) && $size > 0 && $size <= self::MAX_EXCERPT_SOURCE_BYTES) {
            $excerpt = str(file_get_contents($path))->limit(400)->toString();
        }

        return [
            'path' => $path,
            'name' => basename($path),
            'extension' => $extension ?: '(none)',
            'size' => $size,
            'modified_at' => date('Y-m-d', filemtime($path)),
            'excerpt' => $excerpt,
        ];
    }
}
