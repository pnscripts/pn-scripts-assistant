<?php

namespace App\Brain\Tools\Filesystem;

use App\Brain\Tools\Contracts\Tool;
use App\Brain\Tools\Risk;
use RuntimeException;

class ReadFileTool implements Tool
{
    private const MAX_BYTES = 200_000;

    public function name(): string
    {
        return 'read_file';
    }

    public function description(): string
    {
        return 'Read the contents of a text file at an absolute path.';
    }

    public function parameters(): array
    {
        return [
            'type' => 'object',
            'properties' => [
                'path' => ['type' => 'string', 'description' => 'Absolute path to the file'],
            ],
            'required' => ['path'],
        ];
    }

    public function risk(): Risk
    {
        return Risk::Safe;
    }

    public function summarize(array $arguments): string
    {
        return "Read file {$arguments['path']}";
    }

    public function execute(array $arguments): string
    {
        $path = $arguments['path'];

        if (! is_file($path) || ! is_readable($path)) {
            throw new RuntimeException("Not a readable file: {$path}");
        }

        if (filesize($path) > self::MAX_BYTES) {
            // Truncate rather than refuse: a large file's opening is usually
            // still the answer, and blowing the context window helps nobody.
            return substr(file_get_contents($path, length: self::MAX_BYTES), 0, self::MAX_BYTES)
                ."\n\n[truncated at ".self::MAX_BYTES.' bytes]';
        }

        return file_get_contents($path);
    }
}
