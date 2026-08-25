<?php

namespace App\Brain\Integrations\HomeAssistant;

use App\Brain\Integrations\Contracts\Device;
use App\Brain\Integrations\Contracts\DeviceIntegration;
use Illuminate\Support\Facades\Http;
use RuntimeException;

/**
 * Home Assistant, implemented first because it is the one that pays for itself
 * twice: it is open and self-hosted, and it already speaks Zigbee, Z-Wave, Tuya,
 * Hue and hundreds of others. Supporting it supports most of them, so buying
 * hardware that Home Assistant handles is usually wiser than writing a
 * per-vendor adapter here.
 *
 * Configure with HOME_ASSISTANT_URL and HOME_ASSISTANT_TOKEN (a long-lived
 * access token from your HA profile page).
 */
class HomeAssistantIntegration implements DeviceIntegration
{
    /**
     * HA exposes a great deal that is not a controllable device. Restricting to
     * these keeps the model's view small and relevant; widen it when something
     * genuinely useful is missing.
     */
    private const DOMAINS = ['light', 'switch', 'climate', 'sensor', 'binary_sensor', 'cover', 'fan', 'lock'];

    public function __construct(
        private readonly ?string $baseUrl,
        private readonly ?string $token,
    ) {
    }

    public function name(): string
    {
        return 'home-assistant';
    }

    public function isConfigured(): bool
    {
        return ! empty($this->baseUrl) && ! empty($this->token);
    }

    public function devices(): array
    {
        return collect($this->get('/api/states'))
            ->filter(fn (array $s) => in_array($this->domainOf($s['entity_id']), self::DOMAINS, true))
            ->map(fn (array $s) => $this->toDevice($s))
            ->values()
            ->all();
    }

    public function device(string $id): ?Device
    {
        try {
            return $this->toDevice($this->get("/api/states/{$id}"));
        } catch (RuntimeException) {
            return null;
        }
    }

    public function setState(string $id, string $state, array $attributes = []): void
    {
        $domain = $this->domainOf($id);

        // HA is service-oriented rather than state-oriented: you call
        // light.turn_on, you don't assign "on". Translating here keeps that
        // detail out of the tool and out of the model's way.
        $service = match (strtolower($state)) {
            'on', 'turn_on' => 'turn_on',
            'off', 'turn_off' => 'turn_off',
            'toggle' => 'toggle',
            default => throw new RuntimeException(
                "Unsupported state \"{$state}\". Use on, off or toggle."
            ),
        };

        $this->post("/api/services/{$domain}/{$service}", array_merge(
            ['entity_id' => $id],
            $attributes,
        ));
    }

    private function toDevice(array $state): Device
    {
        $attributes = $state['attributes'] ?? [];

        return new Device(
            id: $state['entity_id'],
            name: $attributes['friendly_name'] ?? $state['entity_id'],
            kind: $this->domainOf($state['entity_id']),
            state: $state['state'] ?? null,
            attributes: collect($attributes)
                ->only(['brightness', 'temperature', 'current_temperature', 'unit_of_measurement', 'battery_level'])
                ->all(),
        );
    }

    private function domainOf(string $entityId): string
    {
        return explode('.', $entityId)[0] ?? '';
    }

    private function get(string $path): array
    {
        return $this->request('get', $path);
    }

    private function post(string $path, array $payload): array
    {
        return $this->request('post', $path, $payload);
    }

    private function request(string $method, string $path, array $payload = []): array
    {
        if (! $this->isConfigured()) {
            throw new RuntimeException(
                'Home Assistant is not configured. Set HOME_ASSISTANT_URL and HOME_ASSISTANT_TOKEN.'
            );
        }

        $request = Http::baseUrl(rtrim($this->baseUrl, '/'))
            ->withToken($this->token)
            ->acceptJson()
            ->timeout(15);

        $response = $method === 'get' ? $request->get($path) : $request->post($path, $payload);

        if ($response->failed()) {
            throw new RuntimeException("Home Assistant request failed: HTTP {$response->status()}");
        }

        $json = $response->json();

        // Service calls return a list of changed states; wrap scalars so callers
        // always get an array.
        return is_array($json) ? $json : [];
    }
}
