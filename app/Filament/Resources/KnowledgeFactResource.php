<?php

namespace App\Filament\Resources;

use App\Filament\Resources\KnowledgeFactResource\Pages;
use App\Filament\Resources\KnowledgeFactResource\RelationManagers;
use App\Models\KnowledgeFact;
use Filament\Forms;
use Filament\Forms\Form;
use Filament\Resources\Resource;
use Filament\Tables;
use Filament\Tables\Table;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Database\Eloquent\SoftDeletingScope;

class KnowledgeFactResource extends Resource
{
    protected static ?string $model = KnowledgeFact::class;

    protected static ?string $navigationIcon = 'heroicon-o-academic-cap';

    protected static ?string $navigationLabel = 'Knowledge (long-term)';

    public static function form(Form $form): Form
    {
        return $form
            ->schema([
                Forms\Components\Textarea::make('content')->required()->rows(4)->columnSpanFull(),
                Forms\Components\TextInput::make('category'),
            ]);
    }

    public static function table(Table $table): Table
    {
        return $table
            ->defaultSort('created_at', 'desc')
            ->columns([
                Tables\Columns\TextColumn::make('id')->sortable(),
                Tables\Columns\TextColumn::make('category')->badge()->sortable(),
                Tables\Columns\TextColumn::make('content')->wrap()->limit(140)->searchable(),
                Tables\Columns\TextColumn::make('promoted_from_lesson_id')->label('From lesson')->toggleable(),
                Tables\Columns\TextColumn::make('created_at')->dateTime()->sortable(),
            ])
            ->filters([
                Tables\Filters\SelectFilter::make('category')->options([
                    'project' => 'Project',
                    'document' => 'Document',
                    'conversation' => 'Conversation',
                ]),
            ])
            ->actions([
                Tables\Actions\ViewAction::make(),
                Tables\Actions\EditAction::make(),
            ])
            ->bulkActions([
                Tables\Actions\BulkActionGroup::make([
                    Tables\Actions\DeleteBulkAction::make(),
                ]),
            ]);
    }

    public static function getRelations(): array
    {
        return [
            //
        ];
    }

    public static function getPages(): array
    {
        return [
            'index' => Pages\ListKnowledgeFacts::route('/'),
            'create' => Pages\CreateKnowledgeFact::route('/create'),
            'view' => Pages\ViewKnowledgeFact::route('/{record}'),
            'edit' => Pages\EditKnowledgeFact::route('/{record}/edit'),
        ];
    }
}
