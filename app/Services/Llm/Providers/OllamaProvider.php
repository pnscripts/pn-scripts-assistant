<?php

namespace App\Services\Llm\Providers;

use App\Services\Llm\Contracts\Provider;
use App\Services\Llm\LlmResponse;
use Illuminate\Support\Facades\Http;
use RuntimeException;

class OllamaProvider implements Provider
{
    public function __construct(
        private readonly string $baseUrl,
        private readonly string $model,
    ) {
    }

    public function sendMessage(array $messages): LlmResponse
    {
        $response = Http::baseUrl($this->baseUrl)
            ->timeout(120)
            ->post('/api/chat', [
                'model' => $this->model,
                'messages' => $messages,
                'stream' => false,
            ]);

        if ($response->failed()) {
            throw new RuntimeException("Ollama request failed: {$response->status()} {$response->body()}");
        }

        $content = $response->json('message.content');

        return new LlmResponse(
            content: $content ?? '',
            provider: 'ollama',
            model: $this->model,
        );
    }
}
