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

    public function __construct(private readonly int $conversationId)
    {
    }

    public function handle(LlmRouter $router): void
    {
        $conversation = Conversation::with('messages')->find($this->conversationId);

        if (! $conversation || $conversation->messages->count() < 2) {
            return;
        }

        $transcript = $conversation->messages
            ->map(fn ($m) => "{$m->role}: {$m->content}")
            ->implode("\n");

        $prompt = <<<PROMPT
            You are the Extractor stage of a personal knowledge-capture pipeline.
            Read the exchange below. If it reveals a durable, reusable fact, preference,
            or correction about the user (not one-off task detail), respond with strict
            JSON: {"lesson": "<one sentence>", "confidence": "low"|"medium"|"high"}.
            If there is nothing worth remembering, respond with exactly: {"lesson": null}
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

        Lesson::create([
            'conversation_id' => $conversation->id,
            'content' => $json['lesson'],
            'status' => 'proposed',
            'confidence' => in_array($json['confidence'] ?? null, ['low', 'medium', 'high', 'very-high'], true)
                ? $json['confidence']
                : 'low',
        ]);
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
