<?php

namespace App\Filament\Resources\KnowledgeFactResource\Pages;

use App\Filament\Resources\KnowledgeFactResource;
use Filament\Actions;
use Filament\Resources\Pages\ListRecords;

class ListKnowledgeFacts extends ListRecords
{
    protected static string $resource = KnowledgeFactResource::class;

    protected function getHeaderActions(): array
    {
        return [
            Actions\CreateAction::make(),
        ];
    }
}
