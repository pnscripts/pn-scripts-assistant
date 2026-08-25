<?php

namespace App\Brain\Llm\Contracts;

use App\Brain\Llm\LlmResponse;

interface Provider
{
    /**
     * @param  array<int, array<string, mixed>>  $messages
     * @param  array<int, array<string, mixed>>  $tools  Tool definitions; empty means plain chat.
     */
    public function sendMessage(array $messages, array $tools = []): LlmResponse;
}
