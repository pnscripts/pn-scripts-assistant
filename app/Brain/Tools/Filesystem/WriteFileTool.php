<?php

namespace App\Brain\Tools\Filesystem;

use App\Brain\Tools\Contracts\Tool;
use App\Brain\Tools\Risk;
use RuntimeException;

class WriteFileTool implements Tool
{
    public function name(): string
    {
        return 'write_file';
    }

    public function description(): string
    {
        return 'Write text to a file, creating parent folders as needed. Overwrites an existing file.';
    }

    public function parameters(): array
    {
        return [
            'type' => 'object',
            'properties' => [
                'path' => ['type' => 'string', 'description' => 'Absolute path to write to'],
                'content' => ['type' => 'string', 'description' => 'Full file contents'],
            ],
            'required' => ['path', 'content'],
        ];
    }

    public function risk(): Risk
    {
        return Risk::Mutating;
    }

    /**
     * Says whether this overwrites, because "create a file" and "replace the
     * file you spent yesterday on" deserve different answers from the person
     * approving it.
     */
    public function summarize(array $arguments): string
    {
        $path = $arguments['path'] ?? '(no path)';
        $lines = substr_count((string) ($arguments['content'] ?? ''), "\n") + 1;
        $verb = is_file($path) ? 'OVERWRITE' : 'Create';

        return "{$verb} {$path} ({$lines} lines)";
    }

    public function execute(array $arguments): string
    {
        $path = $arguments['path'];

        // Approval is not enough here: a summary saying "write ~/.ssh/authorized_keys"
        // is easy to wave through, and the consequence is not recoverable.
        SensitivePaths::guard($path);

        $directory = dirname($path);

        if (! is_dir($directory) && ! mkdir($directory, 0o755, true) && ! is_dir($directory)) {
            throw new RuntimeException("Could not create directory: {$directory}");
        }

        if (file_put_contents($path, $arguments['content']) === false) {
            throw new RuntimeException("Could not write: {$path}");
        }

        return 'Wrote '.strlen($arguments['content'])." bytes to {$path}";
    }
}
