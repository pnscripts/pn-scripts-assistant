<?php

namespace App\Brain\Agent;

use App\Brain\Llm\LlmRouter;
use App\Brain\Tools\ToolExecutor;
use App\Brain\Tools\ToolRegistry;
use App\Models\Conversation;
use App\Models\Message;
use App\Models\ToolInvocation;

/**
 * Lets the brain actually *use* its capabilities: ask the model, run what it
 * asks for, feed the results back, repeat until it has an answer.
 *
 * The important behaviour is what happens when the model wants to change
 * something. The loop does not ask for forgiveness and it does not block a web
 * request waiting on a human — it **stops**, leaving the request queued for
 * approval, and reports that plainly. Work resumes on a later turn once the
 * decision exists. That keeps the permission gate meaningful: there is no
 * arrangement of model output that turns a pending action into a taken one.
 */
class AgentLoop
{
    /**
     * Cap on model→tool→model round trips in a single turn. Without it, a model
     * that keeps re-reading the same file loops forever; small local models do
     * this readily.
     */
    private const MAX_STEPS = 8;

    public function __construct(
        private readonly LlmRouter $router,
        private readonly ToolRegistry $registry,
        private readonly ToolExecutor $executor,
    ) {
    }

    /**
     * @param  array<int, array<string, mixed>>  $messages  conversation so far
     */
    public function run(Conversation $conversation, array $messages, ?string $provider = null): AgentResult
    {
        $tools = $this->registry->definitions();
        $actionsTaken = [];

        for ($step = 0; $step < self::MAX_STEPS; $step++) {
            $response = $this->router->send($messages, $provider, $tools);

            if (! $response->wantsTools()) {
                return new AgentResult(
                    reply: $response->content,
                    provider: $response->provider,
                    model: $response->model,
                    actionsTaken: $actionsTaken,
                );
            }

            $pending = [];

            foreach ($response->toolCalls as $call) {
                if (! $this->registry->has($call->name)) {
                    // Hallucinated tool names are common; tell the model rather
                    // than failing the turn, so it can correct itself.
                    $messages[] = $this->toolResultMessage($call->id, "No such tool: {$call->name}");

                    continue;
                }

                $invocation = $this->executor->invoke($call->name, $call->arguments, $conversation->id);

                if ($invocation->isPending()) {
                    $pending[] = $invocation;

                    continue;
                }

                $actionsTaken[] = $invocation->summary;

                $result = $invocation->status === 'failed'
                    ? "Error: {$invocation->error}"
                    : (string) $invocation->result;

                $messages[] = $this->toolResultMessage($call->id, $result);

                // Persisted so a later turn still knows what was looked up;
                // truncated because a large file read would otherwise dominate
                // the transcript and every future prompt built from it.
                Message::create([
                    'conversation_id' => $conversation->id,
                    'role' => 'tool',
                    'content' => "[{$invocation->summary}]\n".str($result)->limit(4000),
                ]);
            }

            if ($pending !== []) {
                return $this->awaitingApproval($conversation, $response, $pending, $actionsTaken);
            }
        }

        return new AgentResult(
            reply: 'I stopped after '.self::MAX_STEPS.' steps without reaching an answer. '
                .'Ask me to continue if that was too soon.',
            provider: $provider ?? config('llm.default_provider'),
            model: '',
            actionsTaken: $actionsTaken,
            hitStepLimit: true,
        );
    }

    /**
     * @param  array<int, ToolInvocation>  $pending
     */
    private function awaitingApproval(
        Conversation $conversation,
        $response,
        array $pending,
        array $actionsTaken,
    ): AgentResult {
        $lines = array_map(fn (ToolInvocation $i) => "  • {$i->summary}", $pending);

        $reply = "I need your approval before continuing:\n".implode("\n", $lines)
            ."\n\nApprove or reject these under Actions & approvals, then tell me to continue.";

        // Recorded in the transcript so the conversation reads coherently later,
        // rather than showing a silent gap where the brain stopped.
        Message::create([
            'conversation_id' => $conversation->id,
            'role' => 'assistant',
            'provider' => $response->provider,
            'model' => $response->model,
            'content' => $reply,
        ]);

        return new AgentResult(
            reply: $reply,
            provider: $response->provider,
            model: $response->model,
            pendingApprovals: $pending,
            actionsTaken: $actionsTaken,
        );
    }

    private function toolResultMessage(?string $callId, string $content): array
    {
        return [
            'role' => 'tool',
            'content' => $content,
            'tool_call_id' => $callId,
        ];
    }
}
