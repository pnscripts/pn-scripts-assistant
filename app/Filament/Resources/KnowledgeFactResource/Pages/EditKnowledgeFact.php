<?php

namespace App\Filament\Resources\KnowledgeFactResource\Pages;

use App\Filament\Resources\KnowledgeFactResource;
use Filament\Actions;
use Filament\Resources\Pages\EditRecord;

class EditKnowledgeFact extends EditRecord
{
    protected static string $resource = KnowledgeFactResource::class;

    protected function getHeaderActions(): array
    {
        return [
            Actions\ViewAction::make(),
            Actions\DeleteAction::make(),
        ];
    }
}
