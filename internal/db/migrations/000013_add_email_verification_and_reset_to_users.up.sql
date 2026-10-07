ALTER TABLE users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN email_verification_code_hash TEXT;
ALTER TABLE users ADD COLUMN email_verification_code_expires_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN password_reset_code_hash TEXT;
ALTER TABLE users ADD COLUMN password_reset_code_expires_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN failed_otp_attempts INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN otp_locked_until TIMESTAMPTZ;

-- Every account created before this migration (bootstrap + anything
-- admin-created under #67) is a company-issued or admin-vetted account,
-- not a self-service signup — mark them verified so the work-item-
-- creation gate (requester-only) never blocks a pre-existing account.
UPDATE users SET email_verified = true;
