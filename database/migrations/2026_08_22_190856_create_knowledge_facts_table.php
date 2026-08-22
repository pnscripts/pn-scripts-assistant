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
        Schema::create('knowledge_facts', function (Blueprint $table) {
            $table->id();
            $table->foreignId('promoted_from_lesson_id')->nullable()->constrained('lessons')->nullOnDelete();
            $table->string('category')->nullable();
            $table->text('content');
            $table->timestamps();
        });

        DB::statement('ALTER TABLE knowledge_facts ADD COLUMN embedding vector(768)');
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('knowledge_facts');
    }
};
