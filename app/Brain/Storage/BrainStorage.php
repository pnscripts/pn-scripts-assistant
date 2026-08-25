<?php

namespace App\Brain\Storage;

use Illuminate\Support\Facades\DB;

/**
 * How much room the brain has left.
 *
 * A brain that learns forever eventually outgrows whatever it started on, and
 * the failure mode for a full disk is ugly: Postgres refuses writes and the
 * assistant simply stops working. So this warns while there is still time to
 * act rather than reporting a problem that has already happened.
 *
 * One thing to know before changing this: BRAIN_DATA_ROOT is a *host* path and
 * the application container cannot see it. The data root's contents arrive
 * inside different containers at different mount points — Postgres has its
 * directory, this container has the workspace — so measuring the data root
 * directly fails with "no such file or directory". What works is measuring a
 * path this container can see that lives on the same drive, which is what the
 * workspace mount is.
 */
class BrainStorage
{
    /**
     * Below this share of the drive, extending is worth raising with the user.
     * A fraction rather than a fixed size, because ten gigabytes is roomy on a
     * laptop and nearly nothing on a media drive.
     */
    private const WARN_BELOW_FRACTION = 0.10;

    /** Postgres stops accepting writes well before a disk reaches zero. */
    private const CRITICAL_BELOW_BYTES = 2 * 1024 * 1024 * 1024;

    /** A path inside this container that sits on the brain's drive. */
    private const PROBE_PATH = '/mnt/workspace';

    public function status(): array
    {
        $probe = is_dir(self::PROBE_PATH) ? self::PROBE_PATH : base_path();

        $total = (int) (@disk_total_space($probe) ?: 0);
        $free = (int) (@disk_free_space($probe) ?: 0);
        $fraction = $total > 0 ? $free / $total : 1.0;
        $level = $this->level($free, $fraction);

        return [
            'host_root' => env('BRAIN_DATA_ROOT'),
            'total_bytes' => $total,
            'free_bytes' => $free,
            'used_bytes' => $total - $free,
            'free_fraction' => round($fraction, 4),
            'database_bytes' => $this->databaseSize(),
            'level' => $level,
            'advice' => $this->advice($level),
        ];
    }

    public function isRunningOut(): bool
    {
        return $this->status()['level'] !== 'ok';
    }

    private function level(int $free, float $fraction): string
    {
        return match (true) {
            $free > 0 && $free < self::CRITICAL_BELOW_BYTES => 'critical',
            $fraction < self::WARN_BELOW_FRACTION => 'low',
            default => 'ok',
        };
    }

    private function advice(string $level): ?string
    {
        return match ($level) {
            'critical' => 'This drive is nearly full. Pnexus will stop being able to learn, '
                .'or even reply, once it fills. Connect another drive and extend storage now.',
            'low' => 'This drive is filling up. Connect another drive and extend storage '
                .'before it becomes a problem.',
            default => null,
        };
    }

    /**
     * What the brain's memory actually occupies. Asked of Postgres rather than
     * measured on disk, because its data directory is owned by root and a walk
     * from here would silently report almost nothing.
     */
    private function databaseSize(): int
    {
        try {
            if (DB::getDriverName() === 'pgsql') {
                return (int) DB::selectOne('select pg_database_size(current_database()) as s')->s;
            }

            $file = DB::getConfig('database');

            return is_string($file) && is_file($file) ? (int) filesize($file) : 0;
        } catch (\Throwable) {
            return 0;
        }
    }
}
