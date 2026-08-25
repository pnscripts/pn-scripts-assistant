<?php

namespace App\Providers;

use App\Brain\Integrations\HomeAssistant\HomeAssistantIntegration;
use App\Brain\Integrations\IntegrationRegistry;
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
 * The complete list of what Pnexus can do to your machine and your home.
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

        $this->app->singleton(ToolRegistry::class, function ($app) {
            $registry = new ToolRegistry;
            $devices = $app->make(IntegrationRegistry::class);

            // Safe — observe only, run without asking.
            $registry->register(new ReadFileTool);
            $registry->register(new ListDirectoryTool);
            $registry->register(new FetchUrlTool);
            $registry->register(new WebSearchTool);
            $registry->register(new ListDevicesTool($devices));

            // Mutating — queued for your approval before anything happens.
            $registry->register(new WriteFileTool);
            $registry->register(new SetDeviceStateTool($devices));

            return $registry;
        });
    }
}
