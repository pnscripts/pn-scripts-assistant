<?php

namespace Tests\Feature;

use App\Brain\Tools\Web\FetchUrlTool;
use PHPUnit\Framework\Attributes\DataProvider;
use RuntimeException;
use Tests\TestCase;

/**
 * fetch_url runs without asking permission, which is only defensible because it
 * cannot be pointed at things reachable purely by virtue of running on Petar's
 * machine — the router's admin page, sibling containers, cloud metadata.
 *
 * These cases are cheap to break accidentally (one "just follow redirects"
 * change would do it) and expensive to notice, so they are pinned here.
 */
class WebFetchGuardTest extends TestCase
{
    private FetchUrlTool $tool;

    protected function setUp(): void
    {
        parent::setUp();
        $this->tool = new FetchUrlTool;
    }

    public static function blockedUrls(): array
    {
        return [
            'loopback by name' => ['http://localhost/'],
            'loopback by ip' => ['http://127.0.0.1/'],
            'private class C' => ['http://192.168.1.1/'],
            'private class A' => ['http://10.0.0.1/'],
            'private class B' => ['http://172.16.0.1/'],
            'link-local metadata' => ['http://169.254.169.254/latest/meta-data/'],
            'ipv6 loopback' => ['http://[::1]/'],
            'file scheme' => ['file:///etc/passwd'],
            'gopher scheme' => ['gopher://127.0.0.1/'],
        ];
    }

    #[DataProvider('blockedUrls')]
    public function test_internal_and_non_http_targets_are_refused(string $url): void
    {
        $this->expectException(RuntimeException::class);

        $this->tool->execute(['url' => $url]);
    }

    public function test_it_is_a_safe_tool_and_so_needs_no_approval(): void
    {
        // If this ever flips to Mutating the guards above stop being the only
        // thing standing between a poisoned page and the local network.
        $this->assertFalse($this->tool->risk()->requiresApproval());
    }
}
