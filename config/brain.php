<?php

return [
    // The assistant's identity — shown in clients and baked into its system prompt.
    // Change freely; nothing else depends on the specific value.
    'name' => env('BRAIN_NAME', 'Pnexus'),
    'owner' => env('BRAIN_OWNER', 'Petar'),

    // Fixed in-container mount points (see compose.yaml) — always read-only.
    // The actual host paths live in SCAN_DEV_PROJECTS_PATH / SCAN_HOME_PROJECTS_PATH / SCAN_DOCUMENTS_PATH.
    'scan' => [
        'dev_projects' => '/mnt/scan/dev-projects',
        'home_projects' => '/mnt/scan/home-projects',
        'documents' => '/mnt/scan/documents',
    ],
];
