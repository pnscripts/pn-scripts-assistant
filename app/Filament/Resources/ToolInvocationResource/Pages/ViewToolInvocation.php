<?php

namespace App\Filament\Resources\ToolInvocationResource\Pages;

use App\Filament\Resources\ToolInvocationResource;
use Filament\Actions;
use Filament\Resources\Pages\ViewRecord;

class ViewToolInvocation extends ViewRecord
{
    protected static string $resource = ToolInvocationResource::class;

    protected function getHeaderActions(): array
    {
        return [
            Actions\EditAction::make(),
        ];
    }
}
