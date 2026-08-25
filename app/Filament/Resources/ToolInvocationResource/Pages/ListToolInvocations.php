<?php

namespace App\Filament\Resources\ToolInvocationResource\Pages;

use App\Filament\Resources\ToolInvocationResource;
use Filament\Actions;
use Filament\Resources\Pages\ListRecords;

class ListToolInvocations extends ListRecords
{
    protected static string $resource = ToolInvocationResource::class;

    protected function getHeaderActions(): array
    {
        return [
            Actions\CreateAction::make(),
        ];
    }
}
