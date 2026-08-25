<?php

namespace App\Providers;

use App\Brain\Tools\Filesystem\ListDirectoryTool;
use App\Brain\Tools\Filesystem\ReadFileTool;
use App\Brain\Tools\Filesystem\WriteFileTool;
use App\Brain\Tools\ToolRegistry;
use Illuminate\Support\ServiceProvider;

/**
 * The complete list of what Pnexus can do to your machine.
 *
 * Deliberately one readable list in one file: adding a capability should be a
 * visible, deliberate act, and this is the file to read when asking "what is
 * this thing actually able to touch?"
 */
class ToolServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        $this->app->singleton(ToolRegistry::class, function () {
            $registry = new ToolRegistry;

            // Safe — run without asking.
            $registry->register(new ReadFileTool);
            $registry->register(new ListDirectoryTool);

            // Mutating — queued for your approval.
            $registry->register(new WriteFileTool);

            return $registry;
        });
    }
}
