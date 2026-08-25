<?php

namespace App\Providers;

use App\Brain\Integrations\HomeAssistant\HomeAssistantIntegration;
use App\Brain\Integrations\IntegrationRegistry;
use App\Brain\Host\HostAgent;
use App\Brain\Privacy;
use App\Brain\Tools\Host\RunCommandTool;
use App\Brain\Tools\Devices\ListDevicesTool;
use App\Brain\Tools\Devices\SetDeviceStateTool;
use App\Brain\Tools\Filesystem\ListDirectoryTool;
use App\Brain\Tools\Filesystem\ReadFileTool;
use App\Brain\Tools\Filesystem\WriteFileTool;
use App\Brain\Tools\ToolRegistry;
use App\Brain\Tools\Web\FetchUrlTool;
use App\Brain\Tools\Web\WebSearchTool;
use Illuminate\Support\ServiceProvider;

/**
 * The complete list of what PN Brain can do to your machine and your home.
 *
 * Deliberately one readable list in one file: adding a capability should be a
 * visible, deliberate act, and this is the file to read when asking "what is
 * this thing actually able to touch?"
 */
class ToolServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        $this->app->singleton(IntegrationRegistry::class, function () {
            $registry = new IntegrationRegistry;

            // Home Assistant fronts Zigbee, Z-Wave, Tuya, Hue and hundreds more,
            // so supporting it supports most platforms. Others implement
            // DeviceIntegration and register here; nothing else changes.
            $registry->register(new HomeAssistantIntegration(
                baseUrl: config('integrations.home_assistant.url'),
                token: config('integrations.home_assistant.token'),
            ));

            return $registry;
        });

        $this->app->singleton(HostAgent::class, fn () => new HostAgent(
            baseUrl: config('brain.host_agent.url'),
            token: config('brain.host_agent.token'),
        ));

        $this->app->singleton(ToolRegistry::class, function ($app) {
            $registry = new ToolRegistry;
            $devices = $app->make(IntegrationRegistry::class);

            // Safe — observe only, run without asking.
            $registry->register(new ReadFileTool);
            $registry->register(new ListDirectoryTool);
            // Withheld entirely in private mode. A tool the model cannot see
            // is one it cannot be talked into using by a page it has read.
            if (Privacy::allowsWeb()) {
                $registry->register(new FetchUrlTool);
                $registry->register(new WebSearchTool);
            }
            $registry->register(new ListDevicesTool($devices));

            // Only offered when the daemon is actually running. A tool that
            // always fails is worse than one that isn't there: the model will
            // keep trying it and reporting the failure as if it were an answer.
            $host = $app->make(HostAgent::class);

            if ($host->isAvailable()) {
                $registry->register(new RunCommandTool($host));
            }

            // Mutating — queued for your approval before anything happens.
            $registry->register(new WriteFileTool);
            $registry->register(new SetDeviceStateTool($devices));

            return $registry;
        });
    }
}
