<?php

namespace App\Brain\Integrations\Contracts;

/**
 * One smart-home platform: Home Assistant, Tuya, Hue, Zigbee, whatever comes next.
 *
 * This exists so adding a platform never means touching the brain. PN Brain should
 * not know what a Hue bridge is; it should know that *something* can list
 * devices and change their state. Implement this, register it, done — the tools,
 * the permission gate and the audit trail come along for free.
 *
 * Deliberately small. Every platform has some notion of "things with state you
 * can read and change", and almost nothing beyond that is shared between them,
 * so anything richer here would end up shaped like whichever platform was
 * implemented first.
 */
interface DeviceIntegration
{
    /** Stable identifier, e.g. "home-assistant". */
    public function name(): string;

    /** Whether this platform is configured well enough to be used. */
    public function isConfigured(): bool;

    /** @return array<int, Device> */
    public function devices(): array;

    public function device(string $id): ?Device;

    /**
     * Change a device's state.
     *
     * @param  array<string, mixed>  $attributes  e.g. ['brightness' => 40]
     */
    public function setState(string $id, string $state, array $attributes = []): void;
}
