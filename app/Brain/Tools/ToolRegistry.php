<?php

namespace App\Brain\Tools;

use App\Brain\Tools\Contracts\Tool;
use InvalidArgumentException;

/**
 * The set of capabilities currently available to the brain.
 *
 * Registration is explicit (see ToolServiceProvider) rather than auto-discovered,
 * because "what can this thing do to my machine" should be a list a person can
 * read in one screen, not an emergent property of which files happen to exist.
 */
class ToolRegistry
{
    /** @var array<string, Tool> */
    private array $tools = [];

    public function register(Tool $tool): void
    {
        $this->tools[$tool->name()] = $tool;
    }

    public function get(string $name): Tool
    {
        return $this->tools[$name]
            ?? throw new InvalidArgumentException("Unknown tool: {$name}");
    }

    public function has(string $name): bool
    {
        return isset($this->tools[$name]);
    }

    /** @return array<string, Tool> */
    public function all(): array
    {
        return $this->tools;
    }

    /** Tool definitions in the shape the LLM APIs expect. */
    public function definitions(): array
    {
        return array_values(array_map(fn (Tool $t) => [
            'name' => $t->name(),
            'description' => $t->description(),
            'input_schema' => $t->parameters(),
        ], $this->tools));
    }
}
