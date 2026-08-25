<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        // Every tool the brain runs is recorded here, approved or not. This is
        // both the approval queue and the audit trail: if the brain ever changes
        // something unexpected, this is where you find out what and when.
        Schema::create('tool_invocations', function (Blueprint $table) {
            $table->id();
            $table->foreignId('conversation_id')->nullable()->constrained()->nullOnDelete();
            $table->string('tool');
            $table->json('arguments');
            $table->string('risk');              // safe | mutating
            $table->string('status')->default('pending'); // pending|executed|failed|rejected
            $table->text('summary')->nullable(); // human-readable "what this will do"
            $table->text('result')->nullable();
            $table->text('error')->nullable();
            $table->timestamp('decided_at')->nullable();
            $table->timestamp('executed_at')->nullable();
            $table->timestamps();

            $table->index(['status', 'created_at']);
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('tool_invocations');
    }
};
