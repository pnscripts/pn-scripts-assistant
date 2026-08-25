<?php

namespace App\Http\Controllers;

use App\Brain\Persona;
use App\Brain\Storage\BrainStorage;
use App\Brain\Tools\ToolExecutor;
use App\Brain\Tools\ToolRegistry;
use App\Models\Conversation;
use App\Models\KnowledgeFact;
use App\Models\Lesson;
use App\Models\Message;
use App\Models\ToolInvocation;
use Illuminate\Http\JsonResponse;

/**
 * Everything the interface needs to show what the brain currently is: who it
 * is, what it knows, and what it is waiting on.
 */
class BrainStatusController extends Controller
{
    public function status(ToolRegistry $tools, BrainStorage $storage): JsonResponse
    {
        return response()->json([
            'name' => Persona::name(),
            'owner' => Persona::owner(),
            'memory' => [
                'facts' => KnowledgeFact::count(),
                'pending_lessons' => Lesson::where('status', 'proposed')->count(),
                'conversations' => Conversation::count(),
            ],
            'capabilities' => collect($tools->all())
                ->map(fn ($t) => ['name' => $t->name(), 'risk' => $t->risk()->value])
                ->values(),
            'pending_approvals' => ToolInvocation::where('status', 'pending')->count(),
            'storage' => $storage->status(),
            'provider' => config('llm.default_provider'),
            'model' => config('llm.'.config('llm.default_provider').'.model'),
        ]);
    }

    public function approvals(): JsonResponse
    {
        return response()->json(
            ToolInvocation::where('status', 'pending')
                ->orderBy('id')
                ->get(['id', 'tool', 'summary', 'created_at'])
        );
    }

    public function decide(ToolInvocation $invocation, string $decision, ToolExecutor $executor): JsonResponse
    {
        $result = $decision === 'approve'
            ? $executor->approve($invocation)
            : $executor->reject($invocation);

        return response()->json([
            'id' => $result->id,
            'status' => $result->status,
            'result' => $result->result,
            'error' => $result->error,
        ]);
    }

    /** Recent history, so a reopened window doesn't start from nothing. */
    public function conversation(Conversation $conversation): JsonResponse
    {
        return response()->json([
            'id' => $conversation->id,
            'title' => $conversation->title,
            'messages' => $conversation->messages()
                ->orderBy('id')
                // The persona prompt is scaffolding, not conversation.
                ->whereIn('role', ['user', 'assistant'])
                ->get(['role', 'content', 'provider', 'model', 'created_at']),
        ]);
    }

    public function latestConversation(): JsonResponse
    {
        $conversation = Conversation::latest('id')->first();

        return $conversation
            ? $this->conversation($conversation)
            : response()->json(['id' => null, 'messages' => []]);
    }

    /** What the brain has done lately — the audit trail, surfaced. */
    public function activity(): JsonResponse
    {
        return response()->json(
            ToolInvocation::latest('id')
                ->limit(20)
                ->get(['id', 'tool', 'summary', 'status', 'risk', 'created_at'])
        );
    }

    public function knowledge(): JsonResponse
    {
        return response()->json(
            KnowledgeFact::latest('id')->limit(50)->get(['id', 'category', 'content'])
        );
    }
}
