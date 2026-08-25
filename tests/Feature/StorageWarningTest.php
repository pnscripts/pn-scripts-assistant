<?php

namespace Tests\Feature;

use App\Brain\Storage\BrainStorage;
use PHPUnit\Framework\Attributes\DataProvider;
use ReflectionMethod;
use Tests\TestCase;

/**
 * The thresholds are the whole feature, and they are the part that cannot be
 * checked by looking at a healthy machine: the panel showing "218GB free" today
 * proves only that the happy path renders.
 *
 * A brain that fills its drive does not degrade gracefully — Postgres refuses
 * writes and the assistant stops answering — so the warning has to arrive while
 * there is still room to act on it.
 */
class StorageWarningTest extends TestCase
{
    private function level(int $freeBytes, float $fraction): string
    {
        $method = new ReflectionMethod(BrainStorage::class, 'level');

        return $method->invoke(new BrainStorage, $freeBytes, $fraction);
    }

    public static function cases(): array
    {
        $gb = 1024 ** 3;

        return [
            'roomy drive' => [500 * $gb, 0.55, 'ok'],
            'exactly at the warning line' => [100 * $gb, 0.10, 'ok'],
            'just under the warning line' => [90 * $gb, 0.09, 'low'],
            'nearly full but large drive' => [50 * $gb, 0.02, 'low'],
            'critically low' => [1 * $gb, 0.01, 'critical'],
            // A small drive can be proportionally fine and still have too little
            // left in absolute terms for a database to keep writing.
            'small drive, healthy fraction, too few bytes' => [(int) (1.5 * $gb), 0.30, 'critical'],
        ];
    }

    #[DataProvider('cases')]
    public function test_levels(int $free, float $fraction, string $expected): void
    {
        $this->assertSame($expected, $this->level($free, $fraction));
    }

    public function test_a_warning_carries_advice_and_a_healthy_drive_does_not(): void
    {
        $advice = new ReflectionMethod(BrainStorage::class, 'advice');
        $storage = new BrainStorage;

        $this->assertNull($advice->invoke($storage, 'ok'));

        foreach (['low', 'critical'] as $level) {
            $text = $advice->invoke($storage, $level);

            $this->assertNotNull($text, "{$level} must explain itself");
            $this->assertStringContainsString(
                'drive',
                $text,
                'The advice has to say what to do, not merely that something is wrong.'
            );
        }
    }
}
