<?php

namespace App\Brain\Tools\Contracts;

use App\Brain\Tools\Risk;

/**
 * One capability the brain can use. Anything the brain can *do* — read a file,
 * write code, fetch a page, turn on a lamp — is a Tool, so a single permission
 * gate and audit trail covers every capability rather than each integration
 * inventing its own rules.
 */
interface Tool
{
    /** Stable identifier the model calls, e.g. "read_file". */
    public function name(): string;

    /** What it does, written for the model deciding whether to call it. */
    public function description(): string;

    /** JSON Schema for the arguments. */
    public function parameters(): array;

    public function risk(): Risk;

    /**
     * One line describing what this specific call would do, shown to the user
     * when asking for approval. Must be concrete ("Write 40 lines to /x/y.php"),
     * because a person cannot meaningfully approve a raw JSON blob.
     */
    public function summarize(array $arguments): string;

    /** Do the thing. Returns text for the model to read. */
    public function execute(array $arguments): string;
}
