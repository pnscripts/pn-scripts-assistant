<?php

namespace App\Filament\Resources;

use App\Filament\Resources\ToolInvocationResource\Pages;
use App\Filament\Resources\ToolInvocationResource\RelationManagers;
use App\Brain\Tools\ToolExecutor;
use App\Models\ToolInvocation;
use Filament\Forms;
use Filament\Forms\Form;
use Filament\Resources\Resource;
use Filament\Tables;
use Filament\Tables\Table;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Database\Eloquent\SoftDeletingScope;

class ToolInvocationResource extends Resource
{
    protected static ?string $model = ToolInvocation::class;

    protected static ?string $navigationIcon = 'heroicon-o-shield-check';

    protected static ?string $navigationLabel = 'Actions & approvals';

    /** Surfaces the count of things waiting on you, so approvals aren't missed. */
    public static function getNavigationBadge(): ?string
    {
        $pending = static::getModel()::where('status', 'pending')->count();

        return $pending > 0 ? (string) $pending : null;
    }

    public static function getNavigationBadgeColor(): ?string
    {
        return 'warning';
    }

    public static function form(Form $form): Form
    {
        return $form
            ->schema([
                Forms\Components\TextInput::make('tool')->disabled(),
                Forms\Components\TextInput::make('status')->disabled(),
                Forms\Components\Textarea::make('summary')->disabled()->rows(2)->columnSpanFull(),
                Forms\Components\Textarea::make('result')->disabled()->rows(6)->columnSpanFull(),
                Forms\Components\Textarea::make('error')->disabled()->rows(3)->columnSpanFull(),
            ]);
    }

    public static function table(Table $table): Table
    {
        return $table
            ->defaultSort('created_at', 'desc')
            ->columns([
                Tables\Columns\TextColumn::make('created_at')->dateTime()->sortable(),
                Tables\Columns\TextColumn::make('tool')->badge(),
                Tables\Columns\TextColumn::make('summary')->wrap()->limit(120),
                Tables\Columns\TextColumn::make('risk')
                    ->badge()
                    ->color(fn (string $state) => $state === 'mutating' ? 'warning' : 'gray'),
                Tables\Columns\TextColumn::make('status')
                    ->badge()
                    ->color(fn (string $state) => match ($state) {
                        'pending' => 'warning',
                        'executed' => 'success',
                        'rejected' => 'gray',
                        'failed' => 'danger',
                        default => 'gray',
                    }),
            ])
            ->filters([
                Tables\Filters\SelectFilter::make('status')->options([
                    'pending' => 'Pending approval',
                    'executed' => 'Executed',
                    'rejected' => 'Rejected',
                    'failed' => 'Failed',
                ]),
            ])
            ->actions([
                Tables\Actions\Action::make('approve')
                    ->label('Approve')
                    ->icon('heroicon-o-check')
                    ->color('success')
                    ->visible(fn (ToolInvocation $r) => $r->isPending())
                    // Mutating actions are irreversible once run, so make the
                    // person confirm against the concrete summary, not the row.
                    ->requiresConfirmation()
                    ->modalDescription(fn (ToolInvocation $r) => $r->summary)
                    ->action(fn (ToolInvocation $r) => app(ToolExecutor::class)->approve($r)),
                Tables\Actions\Action::make('reject')
                    ->label('Reject')
                    ->icon('heroicon-o-x-mark')
                    ->color('danger')
                    ->visible(fn (ToolInvocation $r) => $r->isPending())
                    ->action(fn (ToolInvocation $r) => app(ToolExecutor::class)->reject($r)),
                Tables\Actions\ViewAction::make(),
            ])
            ->bulkActions([]);
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
            'index' => Pages\ListToolInvocations::route('/'),
            'create' => Pages\CreateToolInvocation::route('/create'),
            'view' => Pages\ViewToolInvocation::route('/{record}'),
            'edit' => Pages\EditToolInvocation::route('/{record}/edit'),
        ];
    }
}
