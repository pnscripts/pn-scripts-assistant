<?php

return [
    // Which provider handles a request when nothing overrides it.
    'default_provider' => env('LLM_DEFAULT_PROVIDER', 'ollama'),

    'ollama' => [
        'base_url' => env('OLLAMA_BASE_URL', 'http://host.docker.internal:11434'),
        'model' => env('OLLAMA_DEFAULT_MODEL', 'llama3.2:3b'),
    ],

    'anthropic' => [
        'base_url' => 'https://api.anthropic.com/v1',
        'api_key' => env('ANTHROPIC_API_KEY'),
        'model' => env('ANTHROPIC_MODEL', 'claude-sonnet-5'),
    ],
];
