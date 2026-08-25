<?php

namespace App\Brain\Host;

use Illuminate\Support\Facades\Http;
use RuntimeException;

/**
 * The brain's connection to the machine it runs on.
 *
 * PN Brain lives in a container, so "the filesystem" it sees is three mounted
 * folders. The host agent is a small daemon running outside that container; this
 * is the client for it.
 *
 * Being unconfigured is a normal state, not an error. The brain works without
 * the agent — it simply cannot reach beyond its mounts — so every capability
 * that depends on it checks isAvailable() and stays out of the model's sight
 * when it isn't there. Offering a tool that always fails is worse than not
 * offering it.
 */
class HostAgent
{
    public function __construct(
        private readonly ?string $baseUrl,
        private readonly ?string $token,
    ) {
    }

    public function isConfigured(): bool
    {
        return ! empty($this->baseUrl) && ! empty($this->token);
    }

    /**
     * Whether the daemon is actually running, not merely configured.
     *
     * Cached briefly: this is asked every time the tool list is built, and a
     * failing connection costs a full timeout each time.
     */
    public function isAvailable(): bool
    {
        if (! $this->isConfigured()) {
            return false;
        }

        static $checkedAt = 0;
        static $available = false;

        if (time() - $checkedAt < 10) {
            return $available;
        }

        try {
            $available = Http::timeout(2)->get($this->baseUrl.'/health')->successful();
        } catch (\Throwable) {
            $available = false;
        }

        $checkedAt = time();

        return $available;
    }

    public function readFile(string $path): array
    {
        return $this->post('/read', ['path' => $path]);
    }

    public function listDirectory(string $path): array
    {
        return $this->post('/list', ['path' => $path]);
    }

    /**
     * @param  array<int, string>  $argv  command and arguments, never a shell string
     */
    public function run(array $argv, ?string $dir = null): array
    {
        // A long build or install should not be cut off by an HTTP timeout that
        // is shorter than the agent's own limit; the agent decides when a
        // command has run too long.
        return $this->post('/exec', ['argv' => $argv, 'dir' => $dir], timeout: 120);
    }

    private function post(string $path, array $body, int $timeout = 30): array
    {
        if (! $this->isConfigured()) {
            throw new RuntimeException(
                'The host agent is not configured, so PN Brain cannot reach beyond its own container. '
                .'Set PN_BRAIN_AGENT_URL and PN_BRAIN_AGENT_TOKEN, and start pn-brain-hostagent.'
            );
        }

        try {
            $response = Http::withToken($this->token)
                ->timeout($timeout)
                ->post($this->baseUrl.$path, $body);
        } catch (\Throwable $e) {
            throw new RuntimeException('The host agent is not responding: '.$e->getMessage());
        }

        $json = $response->json() ?? [];

        if ($response->failed()) {
            throw new RuntimeException($json['error'] ?? "Host agent returned {$response->status()}");
        }

        return $json;
    }
}
