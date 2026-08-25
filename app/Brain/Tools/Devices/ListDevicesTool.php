<?php

namespace App\Brain\Tools\Devices;

use App\Brain\Integrations\IntegrationRegistry;
use App\Brain\Tools\Contracts\Tool;
use App\Brain\Tools\Risk;

class ListDevicesTool implements Tool
{
    public function __construct(private readonly IntegrationRegistry $registry)
    {
    }

    public function name(): string
    {
        return 'list_devices';
    }

    public function description(): string
    {
        return 'List smart-home devices and their current state. '
            .'Device ids are "platform:id" and are what set_device_state expects.';
    }

    public function parameters(): array
    {
        return ['type' => 'object', 'properties' => (object) [], 'required' => []];
    }

    public function risk(): Risk
    {
        return Risk::Safe;
    }

    public function summarize(array $arguments): string
    {
        return 'List smart-home devices';
    }

    public function execute(array $arguments): string
    {
        if (! $this->registry->hasAnyConfigured()) {
            return 'No smart-home platform is connected yet. '
                .'Set HOME_ASSISTANT_URL and HOME_ASSISTANT_TOKEN to connect Home Assistant.';
        }

        $devices = $this->registry->allDevices();

        if ($devices === []) {
            return 'Connected, but the platform reported no devices.';
        }

        return collect($devices)
            ->map(fn (array $row) => "{$row['platform']}:{$row['device']->describe()}")
            ->implode("\n");
    }
}
