<?php

namespace App\Brain\Tools\Host;

use App\Brain\Host\HostAgent;
use App\Brain\Tools\Contracts\Tool;
use App\Brain\Tools\Risk;

/**
 * Runs a command on the machine itself.
 *
 * This is the most dangerous capability in PN Brain, and it is shaped so that
 * the dangerous parts are the ones a person actually sees.
 *
 * The command is an array — program first, then arguments — never a shell
 * string. There is no shell, so there is nothing to inject into: a semicolon is
 * an argument, not a second command. It also makes the approval prompt
 * meaningful, because what is shown is exactly what will run, with no expansion
 * or substitution happening afterwards.
 */
class RunCommandTool implements Tool
{
    public function __construct(private readonly HostAgent $agent)
    {
    }

    public function name(): string
    {
        return 'run_command';
    }

    public function description(): string
    {
        return 'Run a command on the computer. Give the program and its arguments as separate '
            .'array items, not a single shell string. There is no shell, so pipes, redirects '
            .'and wildcards will be passed through as literal text.';
    }

    public function parameters(): array
    {
        return [
            'type' => 'object',
            'properties' => [
                'argv' => [
                    'type' => 'array',
                    'items' => ['type' => 'string'],
                    'description' => 'Program then arguments, e.g. ["git","status"]',
                ],
                'dir' => [
                    'type' => 'string',
                    'description' => 'Optional working directory',
                ],
            ],
            'required' => ['argv'],
        ];
    }

    public function risk(): Risk
    {
        return Risk::Mutating;
    }

    /**
     * Shows the exact command. A person approving this is being asked to vouch
     * for something that will run on their machine, so paraphrasing it would
     * make the approval worthless.
     */
    public function summarize(array $arguments): string
    {
        $argv = $arguments['argv'] ?? [];
        $command = implode(' ', array_map(fn ($a) => str_contains((string) $a, ' ') ? '"'.$a.'"' : $a, (array) $argv));
        $dir = $arguments['dir'] ?? null;

        return 'Run: '.$command.($dir ? "  (in {$dir})" : '');
    }

    public function execute(array $arguments): string
    {
        $result = $this->agent->run((array) $arguments['argv'], $arguments['dir'] ?? null);

        $output = trim((string) ($result['output'] ?? ''));
        $exit = $result['exit_code'] ?? 0;

        if (isset($result['error'])) {
            return "The command did not complete: {$result['error']}\n\n{$output}";
        }

        // A non-zero exit is reported rather than thrown. It is usually the
        // answer — a failing test, a dirty working tree — and the model needs to
        // read it to say anything useful.
        $prefix = $exit === 0 ? '' : "Exited with code {$exit}.\n\n";

        return $prefix.($output === '' ? '(no output)' : $output);
    }
}
