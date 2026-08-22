<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class Lesson extends Model
{
    protected $fillable = ['conversation_id', 'content', 'status', 'confidence'];

    public function conversation(): BelongsTo
    {
        return $this->belongsTo(Conversation::class);
    }
}
