<?php

namespace App\Services;

interface MailerInterface
{
    public function send(string $to, string $subject, string $body): void;
}
