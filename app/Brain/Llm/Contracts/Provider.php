<?php

namespace App\Brain\Llm\Contracts;

use App\Brain\Llm\LlmResponse;

interface Provider
{
    /**
     * @param  array<int, array{role: string, content: string}>  $messages
     */
    public function sendMessage(array $messages): LlmResponse;
}
