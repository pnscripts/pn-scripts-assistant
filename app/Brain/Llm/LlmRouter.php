<?php

namespace App\Brain\Llm;

use App\Brain\Llm\Contracts\Provider;
use App\Brain\Llm\Providers\AnthropicProvider;
use App\Brain\Llm\Providers\OllamaProvider;
use InvalidArgumentException;

class LlmRouter
{
    /**
     * @param  array<int, array{role: string, content: string}>  $messages
     * @param  string|null  $provider  Explicit override ('ollama'|'anthropic'). Falls back to config default.
     */
    public function send(array $messages, ?string $provider = null): LlmResponse
    {
        return $this->provider($provider ?? config('llm.default_provider'))->sendMessage($messages);
    }

    public function provider(string $name): Provider
    {
        return match ($name) {
            'ollama' => new OllamaProvider(
                baseUrl: config('llm.ollama.base_url'),
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
