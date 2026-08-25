<?php

namespace App\Brain\Llm;

final readonly class LlmResponse
{
    /**
     * @param  array<int, ToolCall>  $toolCalls
     */
    public function __construct(
        public string $content,
        public string $provider,
        public string $model,
        public array $toolCalls = [],
    ) {
    }

    public function wantsTools(): bool
    {
        return $this->toolCalls !== [];
    }
}
