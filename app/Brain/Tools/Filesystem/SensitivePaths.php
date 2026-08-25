<?php

namespace App\Brain\Tools\Filesystem;

use RuntimeException;

/**
 * Files the brain must never read, whatever it is asked.
 *
 * The reason is a specific attack, not general caution. Reading and fetching are
 * both classed Safe and so run without asking — reasonably, since neither
 * changes anything. Together they are an exfiltration route: a web page can
 * contain text instructing the model to read a credentials file and then fetch
 * a URL with the contents attached, and because neither step needs approval, no
 * human sees it happen.
 *
 * fetch_url already refuses private addresses, which stops the brain being
 * pointed at the local network. This is the other half: stopping secrets from
 * leaving in the first place.
 *
 * Blocking here rather than in the prompt is deliberate. A system prompt is a
 * request; this is a rule. The model cannot be talked out of it, and neither
 * can a page it reads.
 */
class SensitivePaths
{
    /** Matched against the file name alone. */
    private const NAMES = [
        '.env', '.env.local', '.env.production', '.env.backup',
        '.npmrc', '.netrc', '.git-credentials', '.pgpass', '.my.cnf',
        'id_rsa', 'id_ed25519', 'id_ecdsa', 'id_dsa',
        'credentials', 'auth.json', 'shadow', 'passwd-',
    ];

    /** Matched against any part of the path. */
    private const DIRECTORIES = [
        '/.ssh/', '/.gnupg/', '/.aws/', '/.azure/', '/.kube/',
        '/.docker/', '/.config/gcloud/', '/.password-store/',
        '/etc/shadow', '/.mozilla/', '/.thunderbird/',
    ];

    /** Matched against the extension. */
    private const EXTENSIONS = ['pem', 'key', 'p12', 'pfx', 'keystore', 'jks'];

    public static function guard(string $path): void
    {
        if (! self::isSensitive($path)) {
            return;
        }

        // Says what was refused and why, so a legitimate request produces an
        // explanation rather than a puzzle.
        throw new RuntimeException(
            "Refusing to read {$path}: it looks like it holds credentials. ".
            'Secrets are off limits to the assistant even when asked directly.'
        );
    }

    public static function isSensitive(string $path): bool
    {
        $normalised = str_replace('\\', '/', $path);
        $name = basename($normalised);
        $lowerName = strtolower($name);

        if (in_array($lowerName, self::NAMES, true)) {
            return true;
        }

        // Catches .env.whatever without listing every variant.
        if (str_starts_with($lowerName, '.env')) {
            return true;
        }

        if (in_array(strtolower(pathinfo($normalised, PATHINFO_EXTENSION)), self::EXTENSIONS, true)) {
            return true;
        }

        foreach (self::DIRECTORIES as $fragment) {
            if (str_contains($normalised, $fragment)) {
                return true;
            }
        }

        return false;
    }
}
