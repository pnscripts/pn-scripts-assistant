<?php

namespace App\Brain\Tools\Devices;

use App\Brain\Integrations\IntegrationRegistry;
use App\Brain\Tools\Contracts\Tool;
use App\Brain\Tools\Risk;

class SetDeviceStateTool implements Tool
{
    public function __construct(private readonly IntegrationRegistry $registry)
    {
    }

    public function name(): string
    {
        return 'set_device_state';
    }

    public function description(): string
    {
        return 'Turn a smart-home device on, off, or toggle it. '
            .'Use list_devices first to get the device id.';
    }

    public function parameters(): array
    {
        return [
            'type' => 'object',
            'properties' => [
                'device_id' => ['type' => 'string', 'description' => 'Qualified id, e.g. "home-assistant:light.kitchen"'],
                'state' => ['type' => 'string', 'enum' => ['on', 'off', 'toggle']],
                'attributes' => [
                    'type' => 'object',
                    'description' => 'Optional extras, e.g. {"brightness": 128}',
                ],
            ],
            'required' => ['device_id', 'state'],
        ];
    }

    /**
     * Mutating, and not as a formality. Switching something on in the physical
     * world is the least reversible thing in this system: a heater left on or a
     * lock opened has consequences no undo button reaches. Reading device state
     * is free; changing it asks first.
     */
    public function risk(): Risk
    {
        return Risk::Mutating;
    }

    public function summarize(array $arguments): string
    {
        $id = $arguments['device_id'] ?? '(none)';
        $state = $arguments['state'] ?? '(none)';
        $name = $id;

        // Show the human name where possible — "Kitchen light" is something a
        // person can judge; "home-assistant:light.0x00124b" is not.
        try {
            [$integration, $nativeId] = $this->registry->resolve($id);
            $name = $integration->device($nativeId)?->name ?? $id;
        } catch (\Throwable) {
            // Fall back to the raw id; approval must still be possible.
        }

        return "Turn {$state}: {$name}";
    }

    public function execute(array $arguments): string
    {
        [$integration, $nativeId] = $this->registry->resolve($arguments['device_id']);

        $integration->setState(
            $nativeId,
            $arguments['state'],
            $arguments['attributes'] ?? [],
        );

        return "Set {$arguments['device_id']} to {$arguments['state']}.";
    }
}
