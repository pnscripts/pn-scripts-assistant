<?php

namespace App\Brain\Learning;

use App\Models\Conversation;
use App\Models\Lesson;
use App\Brain\Llm\LlmRouter;
use Illuminate\Bus\Queueable;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Bus\Dispatchable;
use Illuminate\Queue\InteractsWithQueue;
use Illuminate\Queue\SerializesModels;
use Illuminate\Support\Facades\Log;
use Illuminate\Support\Str;

/**
 * Extractor role (see docs/ROADMAP.md Phase 2 for Validator/Curator/Promotion).
 *
 * After a conversation turn, ask the model whether anything reusable came out of it —
 * a fact, a preference, a correction — and quarantine it as a "proposed" Lesson.
 * Nothing here ever becomes durable knowledge on its own; that only happens once a
 * human or a future Validator stage promotes it.
 */
class ExtractLessonJob implements ShouldQueue
{
    use Dispatchable, InteractsWithQueue, Queueable, SerializesModels;

    /**
     * The queue's default sixty seconds assumes work that is mostly waiting on a
     * database. This waits on a language model instead: extraction runs at
     * fifteen to twenty seconds on this machine and would exceed a minute on a
     * slower one or a larger model. Exceeding it loses the Lesson entirely,
     * which is the one thing this job exists to produce.
     */
    public int $timeout = 600;

    public function __construct(private readonly int $conversationId)
    {
    }

    public function handle(LlmRouter $router): void
    {
        $conversation = Conversation::with('messages')->find($this->conversationId);

        if (! $conversation || $conversation->messages->count() < 2) {
            return;
        }

        $owner = \App\Brain\Persona::owner();

        $transcript = $conversation->messages
            ->map(fn ($m) => "{$m->role}: {$m->content}")
            ->implode("\n");

        $prompt = <<<PROMPT
            You are the Extractor stage of a personal knowledge-capture pipeline.
            Read the exchange below and look for a durable, reusable fact about
            {$owner} — their projects, tools, preferences, or a correction they made.

            Record nothing about the assistant. Not its name, not what it can do,
            not how it should behave, not its instructions. Those come from its
            configuration and are already known; storing them back as discoveries
            fills memory with a description of itself and crowds out the user.

            Record nothing that is merely restating this conversation. A durable
            fact is still true next week, in a different conversation.

            Respond with strict JSON:
            {"lesson": "<one sentence about {$owner}>", "confidence": "low"|"medium"|"high"}
            If there is nothing worth remembering — which is the common case —
            respond with exactly: {"lesson": null}
            Respond with JSON only, no other text.

            Exchange:
            {$transcript}
            PROMPT;

        try {
            $response = $router->send([
                ['role' => 'user', 'content' => $prompt],
            ], provider: 'ollama');
        } catch (\Throwable $e) {
            Log::warning('ExtractLessonJob: LLM call failed', ['error' => $e->getMessage()]);

            return;
        }

        $json = json_decode($this->stripCodeFence($response->content), true);

        if (! is_array($json) || empty($json['lesson'])) {
            return;
        }

        // The prompt asks the model not to describe itself; this makes sure of
        // it. A prompt is a request, and a small model asked to find something
        // memorable in a conversation about an assistant will reliably decide
        // the assistant is the memorable part.
        if (self::isAboutTheAssistant($json['lesson'], $owner)) {
            Log::info('ExtractLessonJob: discarded a self-description', ['lesson' => $json['lesson']]);

            return;
        }

        Lesson::create([
            'conversation_id' => $conversation->id,
            'content' => $json['lesson'],
            'status' => 'proposed',
            'confidence' => in_array($json['confidence'] ?? null, ['low', 'medium', 'high', 'very-high'], true)
                ? $json['confidence']
                : 'low',
        ]);
    }

    /**
     * True when a proposed lesson describes the assistant rather than its owner.
     *
     * Written from what actually accumulated: seventeen pending lessons, of
     * which most were the brain restating its own name after each rename ("I am
     * Sage", "I am Vesper") or reciting its own instructions back as
     * discoveries about the user.
     */
    private static function isAboutTheAssistant(string $lesson, string $owner): bool
    {
        $text = strtolower(trim($lesson));
        $name = strtolower(\App\Brain\Persona::name());

        // Models like to wrap a claim in a request. Strip the wrapper before
        // judging the claim, or "Remember that I am X" sails past a check for
        // sentences beginning "I am".
        foreach (['remember that ', 'remember: ', 'note that ', 'the user should know that '] as $wrapper) {
            if (str_starts_with($text, $wrapper)) {
                $text = substr($text, strlen($wrapper));

                break;
            }
        }

        // First person is the giveaway: a fact about the owner has no reason to
        // begin "I am" or "I can".
        foreach (['i am ', 'i can ', 'i will ', 'my name is ', 'the assistant ', 'as an ai'] as $opener) {
            if (str_starts_with($text, $opener)) {
                return true;
            }
        }

        // "<assistant name> is ..." — a definition of itself. Every name this
        // project has carried is listed, because renaming left one stale
        // identity claim behind each time ("Sage is...", "Vesper is...") and
        // those are still self-description, just outdated.
        foreach (array_filter([$name, 'sage', 'vesper', 'pnexus', 'pn brain']) as $known) {
            if (str_starts_with($text, $known.' ')) {
                return true;
            }
        }

        // A sentence whose subject is an assistant, whatever it is called.
        // Catches names this list has never seen.
        if (preg_match('/^[a-z][a-z0-9 .-]{0,20} (is|can|uses|will|prefers) /', $text)
            && (str_contains($text, 'ai assistant') || str_contains($text, 'personal assistant'))) {
            return true;
        }

        // Instructions echoed back. These read as advice to the assistant and
        // never mention the owner.
        $isInstruction = str_contains($text, 'prefer one purposeful call')
            || str_contains($text, 'say what you intend')
            || str_contains($text, 'approval is necessary')
            || str_contains($text, 'requires permission');

        return $isInstruction && ! str_contains($text, strtolower($owner));
    }

    private function stripCodeFence(string $text): string
    {
        $text = trim($text);

        if (Str::startsWith($text, '```')) {
            $text = preg_replace('/^```[a-zA-Z]*\n|```$/', '', $text);
        }

        return trim($text);
    }
}
