<?php

namespace App\Brain\Tools;

use App\Models\ToolInvocation;
use Throwable;

/**
 * The single point where the brain's intentions become actions.
 *
 * Every tool call goes through here, so the permission rule lives in exactly one
 * place: safe tools run immediately, mutating tools stop and wait for a human.
 * A mutating call is recorded *before* it runs and only executed via approve(),
 * so there is no path from "the model decided to" to "it happened" that skips
 * the record.
 */
class ToolExecutor
{
    public function __construct(private readonly ToolRegistry $registry)
    {
    }

    /**
     * Run a tool, or queue it for approval if it changes anything.
     *
     * Returns the invocation record; check status to see which happened.
     */
    public function invoke(string $toolName, array $arguments, ?int $conversationId = null): ToolInvocation
    {
        $tool = $this->registry->get($toolName);

        $invocation = ToolInvocation::create([
            'conversation_id' => $conversationId,
            'tool' => $tool->name(),
            'arguments' => $arguments,
            'risk' => $tool->risk()->value,
            'status' => 'pending',
            'summary' => $this->safeSummary($tool, $arguments),
        ]);

        if ($tool->risk()->requiresApproval()) {
            return $invocation; // stays pending until a human decides
        }

        return $this->run($invocation);
    }

    /** A human said yes. */
    public function approve(ToolInvocation $invocation): ToolInvocation
    {
        if (! $invocation->isPending()) {
            return $invocation;
        }

        $invocation->update(['decided_at' => now()]);

        return $this->run($invocation);
    }

    /** A human said no. The tool never runs. */
    public function reject(ToolInvocation $invocation): ToolInvocation
    {
        if ($invocation->isPending()) {
            $invocation->update(['status' => 'rejected', 'decided_at' => now()]);
        }

        return $invocation;
    }

    private function run(ToolInvocation $invocation): ToolInvocation
    {
        try {
            $result = $this->registry->get($invocation->tool)->execute($invocation->arguments);

            $invocation->update([
                'status' => 'executed',
                'result' => $result,
                'executed_at' => now(),
            ]);
        } catch (Throwable $e) {
            // Failures are recorded rather than thrown: the model needs to read
            // what went wrong so it can correct course, and the audit trail
            // should show attempts, not just successes.
            $invocation->update([
                'status' => 'failed',
                'error' => $e->getMessage(),
                'executed_at' => now(),
            ]);
        }

        return $invocation->fresh();
    }

    /**
     * A broken summarizer must not block the approval prompt — falling back to
     * raw arguments is ugly but still lets a person see what was asked for.
     */
    private function safeSummary(Contracts\Tool $tool, array $arguments): string
    {
        try {
            return $tool->summarize($arguments);
        } catch (Throwable) {
            return $tool->name().' '.json_encode($arguments);
        }
    }
}
