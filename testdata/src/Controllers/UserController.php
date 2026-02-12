<?php

namespace App\Controllers;

use App\Services\UserService;

class UserController
{
    private UserService $userService;

    public function __construct(UserService $userService)
    {
        $this->userService = $userService;
    }

    public function index(): void
    {
        // list users
    }

    public function show(int $id): void
    {
        $user = $this->userService->getUser($id);

        if ($user !== null) {
            echo $user->getName();
        }
    }

    public function store(string $name, string $email): void
    {
        if (!UserService::validateEmail($email)) {
            echo "Invalid email";
            return;
        }

        $user = $this->userService->createUser($name, $email);
        echo "Created: " . $user->getName();
    }
}
