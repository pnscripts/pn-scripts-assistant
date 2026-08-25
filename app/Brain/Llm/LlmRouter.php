<?php

namespace App\Brain\Llm;

use App\Brain\Llm\Contracts\Provider;
use App\Brain\Privacy;
use App\Brain\Llm\Providers\AnthropicProvider;
use App\Brain\Llm\Providers\OllamaProvider;
use InvalidArgumentException;

class LlmRouter
{
    /**
     * @param  array<int, array<string, mixed>>  $messages
     * @param  string|null  $provider  Explicit override ('ollama'|'anthropic'). Falls back to config default.
     * @param  array<int, array<string, mixed>>  $tools  Tool definitions; empty means plain chat.
     */
    public function send(array $messages, ?string $provider = null, array $tools = []): LlmResponse
    {
        return $this->provider($provider ?? config('llm.default_provider'))
            ->sendMessage($messages, $tools);
    }

    public function provider(string $name): Provider
    {
        // Enforced here rather than at the call sites, so no future caller can
        // reach a third-party model by forgetting to check first.
        Privacy::guardProvider($name);

        return match ($name) {
            'ollama' => new OllamaProvider(
                baseUrl: config('llm.ollama.base_url'),
                // Tool use is where small models fail hardest, so agent work can
                // point at a stronger local model than everyday chat.
                model: config('llm.ollama.model'),
            ),
            'anthropic' => new AnthropicProvider(
                baseUrl: config('llm.anthropic.base_url'),
                apiKey: config('llm.anthropic.api_key') ?? '',
                model: config('llm.anthropic.model'),
            ),
            default => throw new InvalidArgumentException("Unknown LLM provider: {$name}"),
        };
    }
}
