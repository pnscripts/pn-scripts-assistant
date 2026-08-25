<?php

return [
    // The assistant's identity — shown in clients and baked into its system prompt.
    // Change freely; nothing else depends on the specific value.
    'name' => env('BRAIN_NAME', 'PN Brain'),
    'owner' => env('BRAIN_OWNER', 'Petar'),

    /*
     * What may leave this machine: private | research | open.
     * See App\Brain\Privacy. Defaults to private, because the safe default for
     * a system holding this much personal detail is the one that shares none.
     */
    'privacy' => env('BRAIN_PRIVACY', 'private'),

    // Where the app *reads* these from, which is not the same as where they live
    // on the host: in server mode it sees read-only container mounts, while a
    // desktop build has no container and reads the real paths directly.
    //
    // The defaults are the mount points from compose.yaml, so server mode needs
    // no extra configuration; desktop builds override them with absolute host
    // paths (see .env.desktop.example). SCAN_*_PATH stays the host side of the
    // mount and is consumed by compose.yaml, not by the app.
    /*
     * The host agent: a small daemon running outside the container, which is
     * the only way the brain reaches the machine itself rather than its three
     * mounts. Unset means the brain stays inside its container, which is a
     * valid way to run it.
     */
    'host_agent' => [
        'url' => env('PN_BRAIN_AGENT_URL'),
        'token' => env('PN_BRAIN_AGENT_TOKEN'),
    ],

    'scan' => [
        'dev_projects' => env('SCAN_DEV_PROJECTS_READ_PATH', '/mnt/scan/dev-projects'),
        'home_projects' => env('SCAN_HOME_PROJECTS_READ_PATH', '/mnt/scan/home-projects'),
        'documents' => env('SCAN_DOCUMENTS_READ_PATH', '/mnt/scan/documents'),
    ],
];
