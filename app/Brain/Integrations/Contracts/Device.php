<?php

namespace App\Brain\Integrations\Contracts;

/**
 * One controllable thing, described the same way regardless of which platform
 * it came from, so the model can reason about "the kitchen light" without
 * knowing whether it speaks Zigbee or Wi-Fi.
 */
final readonly class Device
{
    /**
     * @param  string  $id  platform-native identifier
     * @param  string  $name  what a person calls it
     * @param  string  $kind  light, switch, sensor, thermostat, …
     * @param  string|null  $state  current state, if known ("on", "22.5", …)
     * @param  array<string, mixed>  $attributes  brightness, temperature, battery, …
     */
    public function __construct(
        public string $id,
        public string $name,
        public string $kind,
        public ?string $state = null,
        public array $attributes = [],
    ) {
    }

    public function describe(): string
    {
        $line = "{$this->name} ({$this->kind}, id: {$this->id})";

        if ($this->state !== null) {
            $line .= " — {$this->state}";
        }

        return $line;
    }
}
