<?php

namespace App\Brain\Learning;

/**
 * Read-only filesystem scan (see compose.yaml :ro mounts — this never writes
 * anything). Walks a directory tree looking for project markers and stops
 * descending as soon as it finds one, so a project's own vendor/node_modules
 * don't get treated as separate projects.
 */
class ProjectScanner
{
    private const MARKERS = [
        'composer.json' => 'PHP/Composer',
        'package.json' => 'Node.js',
        'go.mod' => 'Go',
        'project.godot' => 'Godot',
        'pyproject.toml' => 'Python',
        'requirements.txt' => 'Python',
        '*.csproj' => 'C#/.NET',
    ];

    private const SKIP_DIRS = [
        'vendor', 'node_modules', '.git', 'storage', 'bootstrap',
        '.next', 'dist', 'build', '.idea', '.vscode', 'target',
        // Third-party code bundled inside a project, not a project of its own.
        'wp-content', 'wp-includes', 'wp-admin',
    ];

    // Matched case-insensitively against the directory name — dated snapshots,
    // not a distinct current project.
    private const SKIP_NAME_PATTERNS = ['/^backup[_-]/i', '/^\.backup/i'];

    private const MAX_DEPTH = 6;

    /** @return array<int, array{path: string, name: string, stack: string, readme_excerpt: ?string, modified_at: string}> */
    public function scan(string $root): array
    {
        $projects = [];
        $this->walk($root, 0, $projects);

        return $projects;
    }

    private function walk(string $dir, int $depth, array &$projects): void
    {
        if ($depth > self::MAX_DEPTH || ! is_dir($dir) || ! is_readable($dir)) {
            return;
        }

        foreach (self::MARKERS as $marker => $stack) {
            $hit = str_contains($marker, '*')
                ? (glob("$dir/$marker") !== false && count(glob("$dir/$marker")) > 0)
                : file_exists("$dir/$marker");

            if ($hit) {
                $projects[] = $this->describe($dir, $stack);

                return;
            }
        }

        $entries = @scandir($dir) ?: [];
        foreach ($entries as $entry) {
            if ($entry === '.' || $entry === '..' || in_array($entry, self::SKIP_DIRS, true)) {
                continue;
            }
            if (collect(self::SKIP_NAME_PATTERNS)->contains(fn ($pattern) => preg_match($pattern, $entry))) {
                continue;
            }
            $path = "$dir/$entry";
            if (is_dir($path) && ! is_link($path)) {
                $this->walk($path, $depth + 1, $projects);
            }
        }
    }

    private function describe(string $dir, string $stack): array
    {
        $readmeExcerpt = null;
        foreach (['README-AI.md', 'README.md', 'readme.md'] as $readme) {
            if (file_exists("$dir/$readme") && is_readable("$dir/$readme")) {
                $readmeExcerpt = str(file_get_contents("$dir/$readme"))->limit(400)->toString();
                break;
            }
        }

        return [
            'path' => $dir,
            'name' => basename($dir),
            'stack' => $stack,
            'readme_excerpt' => $readmeExcerpt,
            'modified_at' => date('Y-m-d', filemtime($dir)),
        ];
    }
}
