<?php

namespace App\Brain;

use RuntimeException;

/**
 * What is allowed to leave this machine.
 *
 * This exists because the brain accumulates an unusually complete picture of
 * one person — every project they own and where it lives, the documents on
 * their disk, what they were doing and when. Sent to a third party, that is not
 * "a prompt", it is a dossier. The sample that prompted this named real client
 * projects and their paths.
 *
 * The rules are enforced here rather than described in the system prompt,
 * because a prompt is a request to a model that can be argued with — by the
 * user, by a web page it reads, or by its own confusion. This cannot.
 *
 * Three levels, and the honesty matters more than the count:
 *
 *  - private  Nothing leaves. Local model, local embeddings, no web. The only
 *             setting under which "no personal information is shared" is a
 *             guarantee rather than an intention.
 *  - research Local model still, so memory and conversation stay put, but the
 *             brain may search and read the web. Search queries do leave, and
 *             the model composes them, so this is a real if narrow disclosure.
 *  - open     A third-party model may be used. Conversation goes to it, and
 *             recalled memory does not — that is withheld at every level
 *             because it is the part the user never chose to type.
 */
class Privacy
{
    public const PRIVATE = 'private';
    public const RESEARCH = 'research';
    public const OPEN = 'open';

    public static function mode(): string
    {
        $mode = strtolower((string) config('brain.privacy', self::PRIVATE));

        return in_array($mode, [self::PRIVATE, self::RESEARCH, self::OPEN], true)
            ? $mode
            // An unrecognised value means a typo in configuration, and the safe
            // reading of a typo is the strict one.
            : self::PRIVATE;
    }

    /** May this provider be used at all? */
    public static function allowsProvider(string $provider): bool
    {
        return $provider === 'ollama' || self::mode() === self::OPEN;
    }

    public static function guardProvider(string $provider): void
    {
        if (self::allowsProvider($provider)) {
            return;
        }

        throw new RuntimeException(
            "Refusing to use the '{$provider}' provider: privacy is set to '".self::mode()."', ".
            'which keeps conversations on this machine. Set BRAIN_PRIVACY=open to allow it.'
        );
    }

    /** May the brain reach the public internet? */
    public static function allowsWeb(): bool
    {
        return self::mode() !== self::PRIVATE;
    }

    /**
     * Whether what the brain has learned may be included in a request to this
     * provider.
     *
     * False for every third party, in every mode. Conversation is typed
     * deliberately; memory is assembled from a person's disk without them
     * composing it, so it is not ours to forward — and it is the richest part.
     */
    public static function allowsMemoryFor(string $provider): bool
    {
        return $provider === 'ollama';
    }

    /** For display: what this setting actually means, in plain words. */
    public static function describe(): array
    {
        return match (self::mode()) {
            self::PRIVATE => [
                'mode' => self::PRIVATE,
                'summary' => 'Nothing leaves this machine.',
                'detail' => 'Local model and local embeddings only. Web search and page '
                    .'fetching are switched off.',
            ],
            self::RESEARCH => [
                'mode' => self::RESEARCH,
                'summary' => 'Your data stays here; the brain may read the web.',
                'detail' => 'Conversations and memory stay on this machine. Search queries '
                    .'the brain composes do reach DuckDuckGo or Brave.',
            ],
            default => [
                'mode' => self::OPEN,
                'summary' => 'Conversations may be sent to a third-party model.',
                'detail' => 'What you type can go to Anthropic. What the brain has learned '
                    .'about you is still withheld.',
            ],
        };
    }
}
