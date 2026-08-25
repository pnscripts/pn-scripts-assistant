<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class ToolInvocation extends Model
{
    protected $fillable = [
        'conversation_id', 'tool', 'arguments', 'risk', 'status',
        'summary', 'result', 'error', 'decided_at', 'executed_at',
    ];

    protected $casts = [
        'arguments' => 'array',
        'decided_at' => 'datetime',
        'executed_at' => 'datetime',
    ];

    public function conversation(): BelongsTo
    {
        return $this->belongsTo(Conversation::class);
    }

    public function isPending(): bool
    {
        return $this->status === 'pending';
    }
}
