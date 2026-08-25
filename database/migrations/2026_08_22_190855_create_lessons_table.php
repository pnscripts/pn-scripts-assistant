<?php

use App\Support\Schema\VectorColumn;
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
        VectorColumn::enableExtension();

        Schema::create('lessons', function (Blueprint $table) {
            $table->id();
            $table->foreignId('conversation_id')->nullable()->constrained()->nullOnDelete();
            $table->text('content');
            $table->enum('status', ['proposed', 'validated', 'promoted', 'rejected'])->default('proposed');
            $table->enum('confidence', ['low', 'medium', 'high', 'very-high'])->nullable();
            $table->timestamps();
        });

        // Real vector type on Postgres, JSON text on SQLite — see VectorColumn.
        VectorColumn::add('lessons');
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('lessons');
    }
};
