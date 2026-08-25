<?php

namespace App\Brain\Llm;

use Illuminate\Support\Facades\Http;
use RuntimeException;

/**
 * Turns text into a vector so the brain can find related memories by meaning
 * rather than keyword. Always local (Ollama) — embeddings run on every stored
 * memory, so sending them to a paid API would be both costly and a needless
 * disclosure of everything the brain knows.
 */
class EmbeddingService
{
    public function embed(string $text): array
    {
        $response = Http::baseUrl(config('llm.ollama.base_url'))
            ->timeout(120)
            ->post('/api/embeddings', [
                'model' => config('llm.embedding.model'),
                'prompt' => $text,
            ]);

        if ($response->failed()) {
            throw new RuntimeException("Embedding request failed: {$response->status()} {$response->body()}");
        }

        $vector = $response->json('embedding');

        if (! is_array($vector) || $vector === []) {
            throw new RuntimeException('Embedding response contained no vector.');
        }

        return $vector;
    }
}
