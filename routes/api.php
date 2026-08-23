<?php

use App\Brain\Persona;
use App\Http\Controllers\ChatController;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Route;

Route::get('/user', function (Request $request) {
    return $request->user();
})->middleware('auth:sanctum');

Route::post('/chat', [ChatController::class, 'send']);

// Lets every client (CLI, web, future desktop/mobile/HUD) introduce the brain
// the same way, without hardcoding its name anywhere but here.
Route::get('/brain', fn () => response()->json([
    'name' => Persona::name(),
    'owner' => Persona::owner(),
]));
