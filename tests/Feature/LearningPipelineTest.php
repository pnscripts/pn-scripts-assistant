<?php

namespace Tests\Feature;

use App\Brain\Learning\Validator;
use App\Models\Lesson;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Tests\TestCase;

/**
 * The Validator decides what is allowed to become permanent knowledge, so its
 * central distinction — verifiable observation versus model inference — is the
 * thing worth pinning down.
 *
 * If chat-derived Lessons ever start auto-validating, a small local model gets
 * to write its own guesses into long-term memory as fact, and the brain starts
 * confidently repeating things nobody ever said.
 */
class LearningPipelineTest extends TestCase
{
    use RefreshDatabase;

    private Validator $validator;

    protected function setUp(): void
    {
        parent::setUp();
        $this->validator = new Validator;
    }

    public function test_filesystem_observations_are_validated_against_reality(): void
    {
        $lesson = Lesson::create([
            'source' => 'project:'.base_path(),
            'content' => 'A project exists here.',
            'status' => 'proposed',
        ]);

        $this->assertSame('validated', $this->validator->statusFor($lesson));
    }

    public function test_observations_of_things_that_no_longer_exist_are_rejected(): void
    {
        $lesson = Lesson::create([
            'source' => 'document:/nonexistent/deleted-yesterday.pdf',
            'content' => 'A document exists here.',
            'status' => 'proposed',
        ]);

        $this->assertSame(
            'rejected',
            $this->validator->statusFor($lesson),
            'A path that no longer exists must not become permanent knowledge.'
        );
    }

    public function test_model_inferences_are_held_for_a_human(): void
    {
        $lesson = Lesson::create([
            'source' => null, // came out of a conversation
            'content' => 'Petar prefers Laravel over Python.',
            'status' => 'proposed',
        ]);

        $this->assertSame(
            'proposed',
            $this->validator->statusFor($lesson),
            'Unverifiable claims must stay quarantined, not auto-promote.'
        );
        $this->assertFalse($this->validator->isMachineVerifiable($lesson));
    }
}
