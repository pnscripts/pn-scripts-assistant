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
        Schema::table('lessons', function (Blueprint $table) {
            // e.g. "project:/mnt/scan/dev-projects/pnscripts" — lets a re-run of an
            // ingestion job skip what it's already proposed, instead of duplicating it.
            $table->string('source')->nullable()->after('conversation_id');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::table('lessons', function (Blueprint $table) {
            $table->dropColumn('source');
        });
    }
};
