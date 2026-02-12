<?php

namespace App\Services;

use App\Repositories\UserRepository;
use App\Models\User;

class UserService
{
    private UserRepository $userRepo;
    private LogService $logger;

    public function __construct(UserRepository $userRepo, LogService $logger)
    {
        $this->userRepo = $userRepo;
        $this->logger = $logger;
    }

    public function getUser(int $id): ?User
    {
        $this->logger->info("Fetching user $id");
        $user = $this->userRepo->find($id);

        if ($user === null) {
            $this->logger->warn("User $id not found");
        }

        return $user;
    }

    public function createUser(string $name, string $email): User
    {
        $user = new User($name, $email);
        $this->userRepo->save($user);
        $this->logger->info("Created user: $name");

        NotificationService::sendWelcomeEmail($user);

        return $user;
    }

    public static function validateEmail(string $email): bool
    {
        return filter_var($email, FILTER_VALIDATE_EMAIL) !== false;
    }
}
