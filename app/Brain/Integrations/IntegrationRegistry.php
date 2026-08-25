<?php

namespace App\Brain\Integrations;

use App\Brain\Integrations\Contracts\Device;
use App\Brain\Integrations\Contracts\DeviceIntegration;
use RuntimeException;

/**
 * Every smart-home platform Pnexus can talk to.
 *
 * Devices are addressed as "platform:id" so two platforms can each have a
 * device called "kitchen" without colliding, and so a device's origin is always
 * visible in the audit trail.
 */
class IntegrationRegistry
{
    /** @var array<string, DeviceIntegration> */
    private array $integrations = [];

    public function register(DeviceIntegration $integration): void
    {
        $this->integrations[$integration->name()] = $integration;
    }

    /** @return array<string, DeviceIntegration> Only those actually set up. */
    public function configured(): array
    {
        return array_filter($this->integrations, fn (DeviceIntegration $i) => $i->isConfigured());
    }

    public function hasAnyConfigured(): bool
    {
        return $this->configured() !== [];
    }

    /** @return array<int, array{platform: string, device: Device}> */
    public function allDevices(): array
    {
        $devices = [];

        foreach ($this->configured() as $name => $integration) {
            foreach ($integration->devices() as $device) {
                $devices[] = ['platform' => $name, 'device' => $device];
            }
        }

        return $devices;
    }

    /** @return array{0: DeviceIntegration, 1: string} the platform and its native id */
    public function resolve(string $qualifiedId): array
    {
        if (! str_contains($qualifiedId, ':')) {
            throw new RuntimeException(
                "Device id must be \"platform:id\", e.g. \"home-assistant:light.kitchen\". Got \"{$qualifiedId}\"."
            );
        }

        [$platform, $nativeId] = explode(':', $qualifiedId, 2);

        $integration = $this->configured()[$platform] ?? throw new RuntimeException(
            "No configured integration named \"{$platform}\"."
        );

        return [$integration, $nativeId];
    }
}
