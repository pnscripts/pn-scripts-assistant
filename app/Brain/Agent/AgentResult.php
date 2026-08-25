<?php

namespace App\Brain\Agent;

final readonly class AgentResult
{
    /**
     * @param  array<int, \App\Models\ToolInvocation>  $pendingApprovals
     * @param  array<int, string>  $actionsTaken  summaries of tools that actually ran
     */
    public function __construct(
        public string $reply,
        public string $provider,
        public string $model,
        public array $pendingApprovals = [],
        public array $actionsTaken = [],
        public bool $hitStepLimit = false,
    ) {
    }

    public function isWaitingForApproval(): bool
    {
        return $this->pendingApprovals !== [];
    }
}
