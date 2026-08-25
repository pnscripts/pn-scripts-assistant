<?php

namespace App\Brain\Tools;

/**
 * How dangerous a tool is, which decides whether it runs on its own.
 *
 * The line is drawn at *observable effect*, not at how scary the name sounds:
 * anything that leaves the world different than it found it needs a human to
 * say yes first. Reading is reversible; writing, deleting, spending, sending
 * and switching things on are not.
 */
enum Risk: string
{
    /** Observes only. Runs without asking. */
    case Safe = 'safe';

    /** Changes something — file, device, remote state. Requires approval. */
    case Mutating = 'mutating';

    public function requiresApproval(): bool
    {
        return $this === self::Mutating;
    }
}
