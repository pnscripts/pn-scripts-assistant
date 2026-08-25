<?php

namespace Tests\Feature;

use App\Brain\Tools\Contracts\Tool;
use App\Brain\Tools\Risk;
use App\Brain\Tools\ToolExecutor;
use App\Brain\Tools\ToolRegistry;
use App\Models\ToolInvocation;
use Illuminate\Foundation\Testing\RefreshDatabase;
use RuntimeException;
use Tests\TestCase;

/**
 * The permission gate is the load-bearing safety property of this whole system:
 * an LLM with filesystem, shell and smart-home access is only acceptable
 * because nothing that changes the world runs without a human agreeing first.
 *
 * These tests exist so that property can't be broken quietly by a later change.
 * They assert on a real side effect — whether a spy actually ran — rather than
 * on status strings, because a bug that flips a status is cosmetic while a bug
 * that runs the action is the one that matters.
 */
class PermissionGateTest extends TestCase
{
    use RefreshDatabase;

    private function registerSpy(Risk $risk, string $name): SpyTool
    {
        $spy = new SpyTool($risk, $name);
        $this->app->make(ToolRegistry::class)->register($spy);

        return $spy;
    }

    public function test_safe_tools_run_immediately(): void
    {
        $spy = $this->registerSpy(Risk::Safe, 'spy_safe');

        $invocation = $this->app->make(ToolExecutor::class)->invoke('spy_safe', ['x' => 1]);

        $this->assertTrue($spy->ran, 'A safe tool should run without asking.');
        $this->assertSame('executed', $invocation->status);
    }

    public function test_mutating_tools_do_not_run_until_approved(): void
    {
        $spy = $this->registerSpy(Risk::Mutating, 'spy_mutating');

        $invocation = $this->app->make(ToolExecutor::class)->invoke('spy_mutating', ['x' => 1]);

        $this->assertFalse($spy->ran, 'A mutating tool must not run before approval.');
        $this->assertSame('pending', $invocation->status);
    }

    public function test_approval_executes_the_action(): void
    {
        $spy = $this->registerSpy(Risk::Mutating, 'spy_approve');
        $executor = $this->app->make(ToolExecutor::class);

        $approved = $executor->approve($executor->invoke('spy_approve', []));

        $this->assertTrue($spy->ran);
        $this->assertSame('executed', $approved->status);
        $this->assertNotNull($approved->decided_at);
    }

    public function test_rejection_never_executes_the_action(): void
    {
        $spy = $this->registerSpy(Risk::Mutating, 'spy_reject');
        $executor = $this->app->make(ToolExecutor::class);

        $rejected = $executor->reject($executor->invoke('spy_reject', []));

        $this->assertFalse($spy->ran, 'A rejected tool must never run.');
        $this->assertSame('rejected', $rejected->status);
    }

    /** Approving twice must not perform the action twice. */
    public function test_approving_an_already_decided_action_is_a_no_op(): void
    {
        $spy = $this->registerSpy(Risk::Mutating, 'spy_double');
        $executor = $this->app->make(ToolExecutor::class);

        $invocation = $executor->approve($executor->invoke('spy_double', []));
        $executor->approve($invocation);

        $this->assertSame(1, $spy->runCount, 'Re-approving must not re-run the action.');
    }

    /** A rejected action must not be resurrectable by approving it afterwards. */
    public function test_rejected_actions_cannot_be_approved_later(): void
    {
        $spy = $this->registerSpy(Risk::Mutating, 'spy_revive');
        $executor = $this->app->make(ToolExecutor::class);

        $executor->approve($executor->reject($executor->invoke('spy_revive', [])));

        $this->assertFalse($spy->ran, 'Approving a rejected action must not run it.');
    }

    /** Every attempt is recorded, so the audit trail shows intent, not just success. */
    public function test_every_invocation_is_recorded_before_it_can_run(): void
    {
        $this->registerSpy(Risk::Mutating, 'spy_audit');

        $this->app->make(ToolExecutor::class)->invoke('spy_audit', ['path' => '/tmp/x']);

        $this->assertDatabaseHas('tool_invocations', [
            'tool' => 'spy_audit',
            'risk' => 'mutating',
            'status' => 'pending',
        ]);
    }

    public function test_failures_are_recorded_rather_than_thrown(): void
    {
        $this->app->make(ToolRegistry::class)->register(new SpyTool(Risk::Safe, 'spy_boom', shouldThrow: true));

        $invocation = $this->app->make(ToolExecutor::class)->invoke('spy_boom', []);

        $this->assertSame('failed', $invocation->status);
        $this->assertStringContainsString('boom', (string) $invocation->error);
    }

    public function test_unknown_tools_are_rejected_outright(): void
    {
        $this->expectException(\InvalidArgumentException::class);

        $this->app->make(ToolExecutor::class)->invoke('no_such_tool', []);
        $this->assertSame(0, ToolInvocation::count());
    }
}

/** Records whether it actually ran, which is the only thing worth asserting on. */
class SpyTool implements Tool
{
    public bool $ran = false;

    public int $runCount = 0;

    public function __construct(
        private readonly Risk $risk,
        private readonly string $name,
        private readonly bool $shouldThrow = false,
    ) {
    }

    public function name(): string
    {
        return $this->name;
    }

    public function description(): string
    {
        return 'Test double.';
    }

    public function parameters(): array
    {
        return ['type' => 'object', 'properties' => (object) []];
    }

    public function risk(): Risk
    {
        return $this->risk;
    }

    public function summarize(array $arguments): string
    {
        return 'spy: '.json_encode($arguments);
    }

    public function execute(array $arguments): string
    {
        $this->ran = true;
        $this->runCount++;

        if ($this->shouldThrow) {
            throw new RuntimeException('boom');
        }

        return 'done';
    }
}
