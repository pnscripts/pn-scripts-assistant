<?php

namespace App\Filament\Resources\ConversationResource\RelationManagers;

use Filament\Resources\RelationManagers\RelationManager;
use Filament\Tables;
use Filament\Tables\Table;

class MessagesRelationManager extends RelationManager
{
    protected static string $relationship = 'messages';

    public function table(Table $table): Table
    {
        return $table
            ->recordTitleAttribute('content')
            ->defaultSort('id')
            ->columns([
                Tables\Columns\TextColumn::make('role')->badge(),
                Tables\Columns\TextColumn::make('content')->wrap()->limit(200),
                Tables\Columns\TextColumn::make('provider'),
                Tables\Columns\TextColumn::make('model'),
                Tables\Columns\TextColumn::make('created_at')->dateTime(),
            ]);
    }
}
