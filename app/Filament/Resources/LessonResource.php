<?php

namespace App\Filament\Resources;

use App\Filament\Resources\LessonResource\Pages;
use App\Filament\Resources\LessonResource\RelationManagers;
use App\Models\Lesson;
use Filament\Forms;
use Filament\Forms\Form;
use Filament\Resources\Resource;
use Filament\Tables;
use Filament\Tables\Table;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Database\Eloquent\SoftDeletingScope;

class LessonResource extends Resource
{
    protected static ?string $model = Lesson::class;

    protected static ?string $navigationIcon = 'heroicon-o-rectangle-stack';

    protected static ?string $navigationLabel = 'Lessons (quarantine)';

    public static function form(Form $form): Form
    {
        return $form
            ->schema([
                Forms\Components\Textarea::make('content')
                    ->required()
                    ->rows(3)
                    ->columnSpanFull(),
                Forms\Components\Select::make('status')
                    ->options([
                        'proposed' => 'Proposed',
                        'validated' => 'Validated',
                        'promoted' => 'Promoted',
                        'rejected' => 'Rejected',
                    ])
                    ->required(),
                Forms\Components\Select::make('confidence')
                    ->options([
                        'low' => 'Low',
                        'medium' => 'Medium',
                        'high' => 'High',
                        'very-high' => 'Very high',
                    ]),
                Forms\Components\Select::make('conversation_id')
                    ->relationship('conversation', 'id')
                    ->searchable(),
            ]);
    }

    public static function table(Table $table): Table
    {
        return $table
            ->defaultSort('created_at', 'desc')
            ->columns([
                Tables\Columns\TextColumn::make('id')->sortable(),
                Tables\Columns\TextColumn::make('content')->wrap()->limit(120),
                Tables\Columns\TextColumn::make('status')
                    ->badge()
                    ->color(fn (string $state) => match ($state) {
                        'proposed' => 'gray',
                        'validated' => 'info',
                        'promoted' => 'success',
                        'rejected' => 'danger',
                    }),
                Tables\Columns\TextColumn::make('confidence')
                    ->badge()
                    ->color(fn (?string $state) => match ($state) {
                        'high', 'very-high' => 'success',
                        'medium' => 'warning',
                        default => 'gray',
                    }),
                Tables\Columns\TextColumn::make('conversation_id')->label('Conversation'),
                Tables\Columns\TextColumn::make('created_at')->dateTime()->sortable(),
            ])
            ->filters([
                Tables\Filters\SelectFilter::make('status')
                    ->options([
                        'proposed' => 'Proposed',
                        'validated' => 'Validated',
                        'promoted' => 'Promoted',
                        'rejected' => 'Rejected',
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
            'index' => Pages\ListLessons::route('/'),
            'create' => Pages\CreateLesson::route('/create'),
            'view' => Pages\ViewLesson::route('/{record}'),
            'edit' => Pages\EditLesson::route('/{record}/edit'),
        ];
    }
}
