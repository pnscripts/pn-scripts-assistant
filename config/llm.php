<?php

return [
    // Which provider handles a request when nothing overrides it.
    'default_provider' => env('LLM_DEFAULT_PROVIDER', 'ollama'),

    'ollama' => [
        'base_url' => env('OLLAMA_BASE_URL', 'http://host.docker.internal:11434'),
        /*
         * Tool use, not raw fluency, decides this. Given five tool definitions
         * and asked "say the word: banana", llama3.2:3b called write_file and
         * returned no text at all; asked "what is 2+2" it emitted the JSON for
         * an invented "add" tool as its literal reply. qwen2.5-coder:7b answers
         * both correctly and reaches for tools only when they are wanted.
         *
         * A small model with tools in front of it is worse than the same model
         * without them, because it treats every request as something to act on.
         */
        'model' => env('OLLAMA_DEFAULT_MODEL', 'qwen2.5-coder:7b'),
    ],

    'anthropic' => [
        'base_url' => 'https://api.anthropic.com/v1',
        'api_key' => env('ANTHROPIC_API_KEY'),
        'model' => env('ANTHROPIC_MODEL', 'claude-sonnet-5'),
    ],

    // Always local — every stored memory gets embedded, so this must stay cheap
    // and private. 768 dimensions, matching the vector() columns in the schema.
    'embedding' => [
        'model' => env('EMBEDDING_MODEL', 'nomic-embed-text'),
        'dimensions' => 768,
    ],

    // Optional. Without a key, search falls back to scraping DuckDuckGo's
    // no-JavaScript endpoint: no account needed, but fragile by nature.
    'search' => [
        'brave_key' => env('BRAVE_SEARCH_API_KEY'),
    ],
];
