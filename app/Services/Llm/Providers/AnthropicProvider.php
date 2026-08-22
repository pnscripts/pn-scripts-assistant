<?php

namespace App\Services\Llm\Providers;

use App\Services\Llm\Contracts\Provider;
use App\Services\Llm\LlmResponse;
use Illuminate\Support\Facades\Http;
use RuntimeException;

class AnthropicProvider implements Provider
{
    public function __construct(
        private readonly string $baseUrl,
        private readonly string $apiKey,
        private readonly string $model,
    ) {
    }

    public function sendMessage(array $messages): LlmResponse
    {
        if ($this->apiKey === '') {
            throw new RuntimeException('ANTHROPIC_API_KEY is not set.');
        }

        // Anthropic takes the system prompt as a top-level field, not a message role.
        $system = collect($messages)
            ->where('role', 'system')
            ->pluck('content')
            ->implode("\n\n");

        $turns = collect($messages)
            ->reject(fn (array $m) => $m['role'] === 'system')
            ->values()
            ->all();

        $payload = [
            'model' => $this->model,
            'max_tokens' => 2048,
            'messages' => $turns,
        ];

        if ($system !== '') {
            $payload['system'] = $system;
        }

        $response = Http::baseUrl($this->baseUrl)
            ->withHeaders([
                'x-api-key' => $this->apiKey,
                'anthropic-version' => '2023-06-01',
            ])
            ->timeout(120)
            ->post('/messages', $payload);

        if ($response->failed()) {
            throw new RuntimeException("Anthropic request failed: {$response->status()} {$response->body()}");
        }

        $content = collect($response->json('content'))
            ->where('type', 'text')
            ->pluck('text')
            ->implode('');

        return new LlmResponse(
            content: $content,
            provider: 'anthropic',
            model: $this->model,
        );
    }
}
