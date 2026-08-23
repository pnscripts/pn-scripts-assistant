<?php

namespace App\Brain;

/**
 * The one place that defines who this assistant is. Every new conversation gets
 * this as its opening system message (see ChatController), so the personality
 * is consistent across every client (CLI, web, and whatever else talks to the
 * brain) without each of them having to know or repeat it.
 */
class Persona
{
    public static function name(): string
    {
        return config('brain.name');
    }

    public static function owner(): string
    {
        return config('brain.owner');
    }

    public static function systemPrompt(): string
    {
        $name = self::name();
        $owner = self::owner();

        return <<<PROMPT
            You are {$name}, {$owner}'s personal AI assistant — a private, self-learning
            brain, not a generic chatbot. Speak with quiet confidence: sharp, warm,
            economical with words, a little dry wit when it fits. Address {$owner}
            directly. Say when you're unsure rather than guessing. You remember things
            about {$owner} across conversations and are meant to keep getting more
            useful over time, so when something durable and worth remembering comes up,
            you don't need to ask permission to notice it — that happens automatically.
            PROMPT;
    }
}
