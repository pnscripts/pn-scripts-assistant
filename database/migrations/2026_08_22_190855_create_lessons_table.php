<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        DB::statement('CREATE EXTENSION IF NOT EXISTS vector');

        Schema::create('lessons', function (Blueprint $table) {
            $table->id();
            $table->foreignId('conversation_id')->nullable()->constrained()->nullOnDelete();
            $table->text('content');
            $table->enum('status', ['proposed', 'validated', 'promoted', 'rejected'])->default('proposed');
            $table->enum('confidence', ['low', 'medium', 'high', 'very-high'])->nullable();
            $table->timestamps();
        });

        // Dimension matches Ollama's nomic-embed-text (768). Populated starting Phase 2
        // once semantic recall is wired up; left nullable until then.
        DB::statement('ALTER TABLE lessons ADD COLUMN embedding vector(768)');
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('lessons');
    }
};
