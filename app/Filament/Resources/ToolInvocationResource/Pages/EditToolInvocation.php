<?php

namespace App\Filament\Resources\ToolInvocationResource\Pages;

use App\Filament\Resources\ToolInvocationResource;
use Filament\Actions;
use Filament\Resources\Pages\EditRecord;

class EditToolInvocation extends EditRecord
{
    protected static string $resource = ToolInvocationResource::class;

    protected function getHeaderActions(): array
    {
        return [
            Actions\ViewAction::make(),
            Actions\DeleteAction::make(),
        ];
    }
}
