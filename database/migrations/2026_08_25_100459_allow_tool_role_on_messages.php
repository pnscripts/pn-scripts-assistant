<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

/**
 * The agent loop records which tools were called and what they returned, so
 * those exchanges need a role of their own — otherwise the model loses
 * everything it looked up as soon as the turn ends.
 *
 * The column becomes a plain string rather than gaining a fourth enum value:
 * the valid set is dictated by whatever LLM providers support, that set keeps
 * growing, and a check constraint here only guarantees another migration every
 * time it does.
 */
return new class extends Migration
{
    public function up(): void
    {
        if (DB::getDriverName() === 'pgsql') {
            // Laravel implements enum() as a varchar plus a check constraint,
            // so dropping the constraint is all that's needed.
            DB::statement('ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_role_check');

            return;
        }

        Schema::table('messages', fn (Blueprint $t) => $t->string('role')->change());
    }

    public function down(): void
    {
        if (DB::getDriverName() !== 'pgsql') {
            return;
        }

        // The constraint cannot be restored while rows violate it.
        DB::statement("DELETE FROM messages WHERE role NOT IN ('user','assistant','system')");
        DB::statement(
            "ALTER TABLE messages ADD CONSTRAINT messages_role_check
             CHECK (role::text = ANY (ARRAY['user','assistant','system']::text[]))"
        );
    }
};
