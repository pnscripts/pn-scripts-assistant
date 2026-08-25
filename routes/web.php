<?php

use Illuminate\Support\Facades\Route;

Route::get('/', fn () => view('console'));

// Kept as a dependency-free fallback for diagnosing the API when the console
// itself is the thing misbehaving.
Route::get('/plain', fn () => view('chat-test'));
