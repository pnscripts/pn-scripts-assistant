<?php

namespace App\Http\Controllers;

use App\Brain\Agent\AgentLoop;
use App\Brain\Learning\ExtractLessonJob;
use App\Brain\Memory\MemoryStore;
use App\Brain\Persona;
use App\Brain\Privacy;
use App\Models\Conversation;
use App\Models\Message;
use App\Models\ToolInvocation;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Log;

class ChatController extends Controller
{
    /**
     * PHP's default 30-second limit is written for requests that query a
     * database and render a page. A reply here waits on a language model, and
     * on a machine without a GPU a single answer can take longer than that on
     * its own — before the agent loop calls a tool and asks again. Hitting the
     * limit surfaces as a bare "Server Error" with nothing in the log, which
     * looks like a crash rather than a timeout.
     *
     * Set per request rather than globally: everything else in this app should
     * still be held to a sensible limit.
     */
    private const REPLY_TIME_LIMIT_SECONDS = 600;

    public function send(Request $request, AgentLoop $agent, MemoryStore $memory): JsonResponse
    {
        set_time_limit(self::REPLY_TIME_LIMIT_SECONDS);

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

        // Memory is only ever given to a local model. What the brain has learned
        // is assembled from this machine's disk — project paths, client names,
        // documents — and the user never composed it, so it is not ours to
        // forward to a third party. What they type is their choice; this is not.
        $provider = $data['provider'] ?? config('llm.default_provider');

        $recalledFacts = Privacy::allowsMemoryFor($provider)
            ? $this->recallFacts($memory, $data['message'])
            : collect();

        $recalled = $this->formatRecall($recalledFacts);

        if ($recalled) {
            array_splice($history, count($history) - 1, 0, [[
                'role' => 'system',
                'content' => $recalled,
            ]]);
        }

        $result = $agent->run($conversation, $history, $data['provider'] ?? null);

        // When the loop stopped for approval it has already written its own
        // message; writing again would duplicate it in the transcript.
        if (! $result->isWaitingForApproval()) {
            Message::create([
                'conversation_id' => $conversation->id,
                'role' => 'assistant',
                'provider' => $result->provider,
                'model' => $result->model,
                'content' => $result->reply,
            ]);
        }

        ExtractLessonJob::dispatch($conversation->id);

        return response()->json([
            'conversation_id' => $conversation->id,
            'reply' => $result->reply,
            'provider' => $result->provider,
            'model' => $result->model,
            'actions_taken' => $result->actionsTaken,
            // Which memories were actually used, so the map can show them
            // lighting up rather than animating at random.
            'recalled' => $recalledFacts->pluck('id')->values(),
            'pending_approvals' => array_map(fn (ToolInvocation $i) => [
                'id' => $i->id,
                'summary' => $i->summary,
            ], $result->pendingApprovals),
        ]);
    }

    /**
     * Recall is a nice-to-have, not a precondition for replying: if the local
     * embedding model is down, the brain should answer without memory rather
     * than fail the whole request.
     */
    private function recallFacts(MemoryStore $memory, string $message)
    {
        try {
            return $memory->recall($message);
        } catch (\Throwable $e) {
            Log::warning('Recall failed; answering without memory.', ['error' => $e->getMessage()]);

            return collect();
        }
    }

    private function formatRecall($facts): ?string
    {
        if ($facts->isEmpty()) {
            return null;
        }

        return "Relevant things you already know, recalled from your own memory:\n".
            $facts->map(fn ($f) => '- '.$f->content)->implode("\n").
            "\n\nUse these if they help. Do not mention this list itself.";
    }
}
