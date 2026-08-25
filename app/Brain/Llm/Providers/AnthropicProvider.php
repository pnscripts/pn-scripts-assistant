<?php

namespace App\Brain\Llm\Providers;

use App\Brain\Llm\Contracts\Provider;
use App\Brain\Llm\LlmResponse;
use App\Brain\Llm\ToolCall;
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

    public function sendMessage(array $messages, array $tools = []): LlmResponse
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
            ->map(fn (array $m) => $this->toAnthropicMessage($m))
            ->values()
            ->all();

        $payload = [
            'model' => $this->model,
            'max_tokens' => 4096,
            'messages' => $turns,
        ];

        if ($system !== '') {
            $payload['system'] = $system;
        }

        // The registry's flat shape is already Anthropic's format.
        if ($tools !== []) {
            $payload['tools'] = $tools;
        }

        $response = Http::baseUrl($this->baseUrl)
            ->withHeaders([
                'x-api-key' => $this->apiKey,
                'anthropic-version' => '2023-06-01',
            ])
            ->timeout(300)
            ->post('/messages', $payload);

        if ($response->failed()) {
            throw new RuntimeException("Anthropic request failed: {$response->status()} {$response->body()}");
        }

        $blocks = collect($response->json('content'));

        return new LlmResponse(
            content: $blocks->where('type', 'text')->pluck('text')->implode(''),
            provider: 'anthropic',
            model: $this->model,
            toolCalls: $blocks->where('type', 'tool_use')
                ->map(fn (array $b) => new ToolCall(
                    name: $b['name'],
                    arguments: $b['input'] ?? [],
                    id: $b['id'] ?? null,
                ))
                ->values()
                ->all(),
        );
    }

    /**
     * Anthropic has no "tool" role: results come back as a user message holding
     * a tool_result block. Internally they're stored with role "tool" so the
     * transcript stays readable and provider-agnostic, and are translated here.
     */
    private function toAnthropicMessage(array $message): array
    {
        if (($message['role'] ?? null) !== 'tool') {
            return ['role' => $message['role'], 'content' => (string) $message['content']];
        }

        return [
            'role' => 'user',
            'content' => [[
                'type' => 'tool_result',
                'tool_use_id' => $message['tool_call_id'] ?? '',
                'content' => (string) $message['content'],
            ]],
        ];
    }
}
