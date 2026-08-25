<?php

namespace App\Brain\Llm;

/**
 * A model's request to use a capability, normalised across providers.
 *
 * Ollama and Anthropic describe tool calls differently; both are translated into
 * this shape by their provider so nothing downstream — the agent loop, the
 * permission gate, the audit trail — has to care which model was driving.
 */
final readonly class ToolCall
{
    public function __construct(
        public string $name,
        public array $arguments,
        /** Provider's own id for the call, needed to match results back to it. */
        public ?string $id = null,
    ) {
    }
}
