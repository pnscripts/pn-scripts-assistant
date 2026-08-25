<?php

namespace Tests\Feature;

use App\Brain\Llm\LlmRouter;
use App\Brain\Privacy;
use App\Brain\Tools\ToolRegistry;
use RuntimeException;
use Tests\TestCase;

/**
 * The brain accumulates an unusually complete picture of one person: every
 * project they own and where it lives, what documents sit on their disk, what
 * they were doing and when. Sent to a third party that is not a prompt, it is a
 * dossier.
 *
 * These are the guarantees that make "no personal information is shared" a
 * property of the code rather than an intention, so they are pinned here. The
 * memory rule especially: it holds in *every* mode, including the most
 * permissive one, because conversation is typed deliberately while memory is
 * assembled from a person's disk without them composing it.
 */
class PrivacyTest extends TestCase
{
    private function withMode(string $mode): void
    {
        config(['brain.privacy' => $mode]);
    }

    public function test_private_mode_is_the_default_when_unset(): void
    {
        config(['brain.privacy' => null]);

        $this->assertSame(Privacy::PRIVATE, Privacy::mode());
    }

    /** A typo in configuration must fail closed, not open. */
    public function test_an_unrecognised_mode_falls_back_to_private(): void
    {
        $this->withMode('publik');

        $this->assertSame(Privacy::PRIVATE, Privacy::mode());
        $this->assertFalse(Privacy::allowsProvider('anthropic'));
    }

    public function test_third_party_models_are_refused_unless_explicitly_opened(): void
    {
        foreach ([Privacy::PRIVATE, Privacy::RESEARCH] as $mode) {
            $this->withMode($mode);

            $this->assertFalse(
                Privacy::allowsProvider('anthropic'),
                "anthropic must be refused in {$mode} mode"
            );
        }

        $this->withMode(Privacy::OPEN);
        $this->assertTrue(Privacy::allowsProvider('anthropic'));
    }

    public function test_the_router_refuses_rather_than_silently_downgrading(): void
    {
        $this->withMode(Privacy::PRIVATE);

        // Silently using a different model would be worse than failing: the user
        // would believe they were talking to the one they asked for.
        $this->expectException(RuntimeException::class);

        app(LlmRouter::class)->provider('anthropic');
    }

    public function test_local_models_are_always_available(): void
    {
        foreach ([Privacy::PRIVATE, Privacy::RESEARCH, Privacy::OPEN] as $mode) {
            $this->withMode($mode);

            $this->assertTrue(Privacy::allowsProvider('ollama'));
        }
    }

    /**
     * The central guarantee. Memory is withheld from third parties in every
     * mode, including open — opening the door to a better model is not consent
     * to hand over everything the brain has quietly learned.
     */
    public function test_memory_never_goes_to_a_third_party_in_any_mode(): void
    {
        foreach ([Privacy::PRIVATE, Privacy::RESEARCH, Privacy::OPEN] as $mode) {
            $this->withMode($mode);

            $this->assertFalse(
                Privacy::allowsMemoryFor('anthropic'),
                "memory must never be sent to a third party, and {$mode} mode allowed it"
            );
            $this->assertTrue(Privacy::allowsMemoryFor('ollama'));
        }
    }

    public function test_private_mode_withholds_web_tools_entirely(): void
    {
        $this->withMode(Privacy::PRIVATE);
        $this->refreshApplication();
        config(['brain.privacy' => Privacy::PRIVATE]);

        $names = array_keys(app(ToolRegistry::class)->all());

        // Absent rather than refused: a tool the model cannot see is one it
        // cannot be argued into using by a page it has read.
        $this->assertNotContains('fetch_url', $names);
        $this->assertNotContains('web_search', $names);
        $this->assertContains('read_file', $names, 'local capabilities must be unaffected');
    }

    public function test_every_mode_can_explain_itself_in_plain_words(): void
    {
        foreach ([Privacy::PRIVATE, Privacy::RESEARCH, Privacy::OPEN] as $mode) {
            $this->withMode($mode);
            $described = Privacy::describe();

            $this->assertSame($mode, $described['mode']);
            $this->assertNotEmpty($described['summary']);
            $this->assertNotEmpty($described['detail']);
        }
    }
}
