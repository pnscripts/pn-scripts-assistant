<?php

namespace App\Http\Controllers;

use App\Models\Conversation;
use App\Models\Message;
use App\Services\Learning\ExtractLessonJob;
use App\Services\Llm\LlmRouter;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;

class ChatController extends Controller
{
    public function send(Request $request, LlmRouter $router): JsonResponse
    {
        $data = $request->validate([
            'conversation_id' => ['nullable', 'integer', 'exists:conversations,id'],
            'message' => ['required', 'string'],
            'provider' => ['nullable', 'string', 'in:ollama,anthropic'],
        ]);

        $conversation = $data['conversation_id'] ?? null
            ? Conversation::findOrFail($data['conversation_id'])
            : Conversation::create(['title' => str($data['message'])->limit(60)]);

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
}
