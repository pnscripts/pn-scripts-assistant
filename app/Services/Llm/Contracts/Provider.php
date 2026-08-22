<?php

namespace App\Services\Llm\Contracts;

use App\Services\Llm\LlmResponse;

interface Provider
{
    /**
     * @param  array<int, array{role: string, content: string}>  $messages
     */
    public function sendMessage(array $messages): LlmResponse;
}
