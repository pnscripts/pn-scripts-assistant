<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class KnowledgeFact extends Model
{
    protected $fillable = ['promoted_from_lesson_id', 'category', 'content'];

    public function promotedFromLesson(): BelongsTo
    {
        return $this->belongsTo(Lesson::class, 'promoted_from_lesson_id');
    }
}
