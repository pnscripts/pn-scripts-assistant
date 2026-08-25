<?php

namespace Tests\Feature;

use App\Brain\Tools\Filesystem\ReadFileTool;
use App\Brain\Tools\Filesystem\SensitivePaths;
use App\Brain\Tools\Filesystem\WriteFileTool;
use PHPUnit\Framework\Attributes\DataProvider;
use RuntimeException;
use Tests\TestCase;

/**
 * Reading a file and fetching a URL are both classed Safe, so both run without
 * asking. Individually that is right — neither changes anything. Together they
 * are an exfiltration route: a fetched page can contain text telling the model
 * to read a credentials file and then request a URL with the contents attached,
 * and no human is ever shown either step.
 *
 * These cases pin the half of that which stops secrets leaving. The other half
 * lives in WebFetchGuardTest, which stops the brain being aimed at the local
 * network.
 */
class SecretExfiltrationTest extends TestCase
{
    public static function secretPaths(): array
    {
        return [
            'app environment' => ['/var/www/html/.env'],
            'env variant' => ['/srv/app/.env.production'],
            'ssh directory' => ['/home/petar/.ssh/id_rsa'],
            'ssh key by name' => ['/tmp/id_ed25519'],
            'aws credentials' => ['/home/petar/.aws/credentials'],
            'gnupg' => ['/home/petar/.gnupg/secring.gpg'],
            'kube config' => ['/home/petar/.kube/config'],
            'private key extension' => ['/etc/ssl/private/server.pem'],
            'certificate key' => ['/opt/certs/tls.key'],
            'composer auth' => ['/var/www/html/auth.json'],
            'npm token' => ['/home/petar/.npmrc'],
            'system shadow' => ['/etc/shadow'],
            'browser profile' => ['/home/petar/.mozilla/firefox/x/logins.json'],
        ];
    }

    #[DataProvider('secretPaths')]
    public function test_credentials_cannot_be_read(string $path): void
    {
        $this->expectException(RuntimeException::class);

        (new ReadFileTool)->execute(['path' => $path]);
    }

    /**
     * Approval is not sufficient protection for these: a one-line summary saying
     * "write ~/.ssh/authorized_keys" is easy to wave through, and the result is
     * not recoverable.
     */
    #[DataProvider('secretPaths')]
    public function test_credentials_cannot_be_written(string $path): void
    {
        $this->expectException(RuntimeException::class);

        (new WriteFileTool)->execute(['path' => $path, 'content' => 'x']);
    }

    public function test_ordinary_files_are_unaffected(): void
    {
        foreach ([
            '/home/petar/Projects/app/composer.json',
            '/mnt/scan/documents/notes.md',
            '/tmp/report.pdf',
            '/var/www/html/README.md',
        ] as $path) {
            $this->assertFalse(
                SensitivePaths::isSensitive($path),
                "{$path} should be readable; over-blocking makes the assistant useless."
            );
        }
    }
}
