<?php

return [
    // The assistant's identity — shown in clients and baked into its system prompt.
    // Change freely; nothing else depends on the specific value.
    'name' => env('BRAIN_NAME', 'Vesper'),
    'owner' => env('BRAIN_OWNER', 'Petar'),
];
