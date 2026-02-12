<?php

namespace App\Repositories;

use App\Models\User;

class UserRepository
{
    public function find(int $id): ?User
    {
        // DB query
        return null;
    }

    public function save(User $user): void
    {
        // DB insert/update
    }

    public function delete(User $user): void
    {
        // DB delete
    }
}
