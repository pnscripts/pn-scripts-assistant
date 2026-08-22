<?php

namespace App\Services\Llm;

final readonly class LlmResponse
{
    public function __construct(
        public string $content,
        public string $provider,
        public string $model,
    ) {
    }
}
