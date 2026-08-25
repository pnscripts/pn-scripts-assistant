<?php

namespace App\Http\Controllers;

use App\Brain\Learning\ExtractLessonJob;
use App\Brain\Llm\LlmRouter;
use App\Brain\Memory\MemoryStore;
use App\Brain\Persona;
use App\Models\Conversation;
use App\Models\Message;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Log;

class ChatController extends Controller
{
    public function send(Request $request, LlmRouter $router, MemoryStore $memory): JsonResponse
    {
        $data = $request->validate([
            'conversation_id' => ['nullable', 'integer', 'exists:conversations,id'],
            'message' => ['required', 'string'],
            'provider' => ['nullable', 'string', 'in:ollama,anthropic'],
        ]);

        $isNewConversation = ! ($data['conversation_id'] ?? null);

        $conversation = $isNewConversation
            ? Conversation::create(['title' => str($data['message'])->limit(60)])
            : Conversation::findOrFail($data['conversation_id']);

        if ($isNewConversation) {
            Message::create([
                'conversation_id' => $conversation->id,
                'role' => 'system',
                'content' => Persona::systemPrompt(),
            ]);
        }

        Message::create([
            'conversation_id' => $conversation->id,
            'role' => 'user',
            'content' => $data['message'],
        ]);

        $history = $conversation->messages()
            ->orderBy('id')
            ->get()
            ->map(fn (Message $m) => ['role' => $m->role, 'content' => $m->content])
            ->all();

        // Pull in anything already known that bears on this message, injected fresh
        // each turn rather than persisted — memory changes between turns, and a stale
        // copy baked into the transcript would go on being repeated after the
        // underlying fact was corrected or removed.
        if ($recalled = $this->recallContext($memory, $data['message'])) {
            array_splice($history, count($history) - 1, 0, [[
                'role' => 'system',
                'content' => $recalled,
            ]]);
        }

        $response = $router->send($history, provider: $data['provider'] ?? null);

        Message::create([
            'conversation_id' => $conversation->id,
            'role' => 'assistant',
            'provider' => $response->provider,
            'model' => $response->model,
            'content' => $response->content,
        ]);

        ExtractLessonJob::dispatch($conversation->id);

        return response()->json([
            'conversation_id' => $conversation->id,
            'reply' => $response->content,
            'provider' => $response->provider,
            'model' => $response->model,
        ]);
    }

    /**
     * Recall is a nice-to-have, not a precondition for replying: if the local
     * embedding model is down, the brain should answer without memory rather
     * than fail the whole request.
     */
    private function recallContext(MemoryStore $memory, string $message): ?string
    {
        try {
            $facts = $memory->recall($message);
        } catch (\Throwable $e) {
            Log::warning('Recall failed; answering without memory.', ['error' => $e->getMessage()]);

            return null;
        }

        if ($facts->isEmpty()) {
            return null;
        }

        return "Relevant things you already know, recalled from your own memory:\n".
            $facts->map(fn ($f) => '- '.$f->content)->implode("\n").
            "\n\nUse these if they help. Do not mention this list itself.";
    }
}
