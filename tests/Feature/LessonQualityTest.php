<?php

namespace Tests\Feature;

use App\Brain\Learning\ExtractLessonJob;
use PHPUnit\Framework\Attributes\DataProvider;
use ReflectionMethod;
use Tests\TestCase;

/**
 * Written from a real backlog. Seventeen lessons had accumulated awaiting
 * review, and most were the brain describing itself: "I am Sage", "I am
 * Vesper", "PN Brain is Petar's personal AI assistant" — one for each time the
 * project was renamed — alongside its own instructions recited back as
 * discoveries ("prefer one purposeful call over several speculative ones").
 *
 * This is a predictable failure rather than a fluke. A small model asked to
 * find something memorable in a conversation with an assistant will decide the
 * assistant is the memorable part. Left alone it crowds out the thing memory is
 * for, and every rename adds another stale identity claim.
 */
class LessonQualityTest extends TestCase
{
    private function isAboutTheAssistant(string $lesson): bool
    {
        $method = new ReflectionMethod(ExtractLessonJob::class, 'isAboutTheAssistant');

        return $method->invoke(null, $lesson, 'Petar');
    }

    public static function selfDescriptions(): array
    {
        return [
            'stale identity' => ['I am Sage, a self-learning AI assistant, not a generic chatbot.'],
            'later identity' => ['I am Vesper, the personal AI assistant for Petar.'],
            'current identity' => ['PN Brain is Petar\'s personal AI assistant'],
            'capability boast' => ['I can recall our conversations and use that to improve over time.'],
            'about the assistant' => ['The assistant can repeat previous statements.'],
            'instruction echo' => ['When using tools, prefer one purposeful call over several speculative ones.'],
            'behaviour echo' => ['If unsure, say what you intend to do and why before taking action.'],
            'model preamble' => ['As an AI assistant, I keep information private.'],
        ];
    }

    #[DataProvider('selfDescriptions')]
    public function test_self_description_is_not_learned(string $lesson): void
    {
        $this->assertTrue(
            $this->isAboutTheAssistant($lesson),
            "This should have been discarded: {$lesson}"
        );
    }

    public static function realFacts(): array
    {
        return [
            'preference' => ['Petar prefers Laravel over Python for backend projects.'],
            'inventory' => ['Petar has two Go projects: docs-saas-golang-api and xplorer-golang-api.'],
            'structure' => ['The xplorer folder contains five projects including xplorer-admin-front.'],
            'correction' => ['Petar corrected that the brain lives on an external drive, not the system disk.'],
            'tooling' => ['Petar uses Filament for admin interfaces in his Laravel projects.'],
        ];
    }

    /**
     * The guard has to stay narrow. Over-blocking would be the worse failure:
     * a brain that learns nothing is less useful than one that occasionally
     * learns something silly, and silence is harder to notice than noise.
     */
    #[DataProvider('realFacts')]
    public function test_genuine_facts_are_kept(string $lesson): void
    {
        $this->assertFalse(
            $this->isAboutTheAssistant($lesson),
            "This is a real fact and must be kept: {$lesson}"
        );
    }

    /** An instruction that genuinely concerns the owner is still worth keeping. */
    public function test_an_instruction_mentioning_the_owner_is_kept(): void
    {
        $this->assertFalse(
            $this->isAboutTheAssistant('Petar asked that approval is necessary before any deployment.')
        );
    }
}
