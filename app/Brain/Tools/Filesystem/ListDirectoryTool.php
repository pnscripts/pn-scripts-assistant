<?php

namespace App\Brain\Tools\Filesystem;

use App\Brain\Tools\Contracts\Tool;
use App\Brain\Tools\Risk;
use RuntimeException;

class ListDirectoryTool implements Tool
{
    private const MAX_ENTRIES = 300;

    public function name(): string
    {
        return 'list_directory';
    }

    public function description(): string
    {
        return 'List the files and folders in a directory at an absolute path.';
    }

    public function parameters(): array
    {
        return [
            'type' => 'object',
            'properties' => [
                'path' => ['type' => 'string', 'description' => 'Absolute path to the directory'],
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
        return "List directory {$arguments['path']}";
    }

    public function execute(array $arguments): string
    {
        $path = rtrim($arguments['path'], '/');

        if (! is_dir($path) || ! is_readable($path)) {
            throw new RuntimeException("Not a readable directory: {$path}");
        }

        $entries = collect(scandir($path))
            ->reject(fn ($e) => $e === '.' || $e === '..')
            ->take(self::MAX_ENTRIES)
            ->map(fn ($e) => is_dir("$path/$e") ? "$e/" : $e)
            ->values();

        return $entries->isEmpty()
            ? '(empty directory)'
            : $entries->implode("\n");
    }
}
