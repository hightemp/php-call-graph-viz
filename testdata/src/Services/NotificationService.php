<?php

namespace App\Services;

use App\Models\User;

class NotificationService
{
    private MailerInterface $mailer;

    public function __construct(MailerInterface $mailer)
    {
        $this->mailer = $mailer;
    }

    public static function sendWelcomeEmail(User $user): void
    {
        $instance = new self();
        $instance->doSendEmail($user, 'Welcome!');
    }

    private function doSendEmail(User $user, string $subject): void
    {
        $this->mailer->send($user->getEmail(), $subject, 'Hello!');
    }
}
