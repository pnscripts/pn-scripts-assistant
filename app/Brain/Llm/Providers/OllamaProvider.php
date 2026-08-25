<?php

namespace App\Brain\Llm\Providers;

use App\Brain\Llm\Contracts\Provider;
use App\Brain\Llm\LlmResponse;
use App\Brain\Llm\ToolCall;
use Illuminate\Support\Facades\Http;
use RuntimeException;

class OllamaProvider implements Provider
{
    public function __construct(
        private readonly string $baseUrl,
        private readonly string $model,
    ) {
    }

    public function sendMessage(array $messages, array $tools = []): LlmResponse
    {
        $payload = [
            'model' => $this->model,
            'messages' => $this->normalizeMessages($messages),
            'stream' => false,
        ];

        if ($tools !== []) {
            // Ollama follows the OpenAI shape, which nests the schema under
            // "function" rather than the flat form the registry emits.
            $payload['tools'] = array_map(fn (array $t) => [
                'type' => 'function',
                'function' => [
                    'name' => $t['name'],
                    'description' => $t['description'],
                    'parameters' => $t['input_schema'],
                ],
            ], $tools);
        }

        // Tool loops make several sequential calls, and CPU-only inference is
        // slow, so this is far longer than a single-turn chat would need.
        $response = Http::baseUrl($this->baseUrl)->timeout(300)->post('/api/chat', $payload);

        if ($response->failed()) {
            throw new RuntimeException("Ollama request failed: {$response->status()} {$response->body()}");
        }

        return new LlmResponse(
            content: $response->json('message.content') ?? '',
            provider: 'ollama',
            model: $this->model,
            toolCalls: $this->parseToolCalls($response->json('message.tool_calls') ?? []),
        );
    }

    /**
     * Messages carry extra metadata internally for the agent loop; Ollama wants
     * only role and content, so anything else is dropped here.
     */
    private function normalizeMessages(array $messages): array
    {
        return array_map(fn (array $m) => [
            'role' => $m['role'],
            'content' => (string) $m['content'],
        ], $messages);
    }

    private function parseToolCalls(array $raw): array
    {
        return array_values(array_filter(array_map(function (array $call) {
            $function = $call['function'] ?? [];
            $name = $function['name'] ?? null;

            if (! $name) {
                return null;
            }

            $arguments = $function['arguments'] ?? [];

            // Smaller models often emit arguments as a JSON string instead of an object.
            if (is_string($arguments)) {
                $arguments = json_decode($arguments, true) ?: [];
            }

            return new ToolCall(name: $name, arguments: $arguments, id: $call['id'] ?? null);
        }, $raw)));
    }
}
