ALTER TABLE users DROP COLUMN IF EXISTS otp_locked_until;
ALTER TABLE users DROP COLUMN IF EXISTS failed_otp_attempts;
ALTER TABLE users DROP COLUMN IF EXISTS password_reset_code_expires_at;
ALTER TABLE users DROP COLUMN IF EXISTS password_reset_code_hash;
ALTER TABLE users DROP COLUMN IF EXISTS email_verification_code_expires_at;
ALTER TABLE users DROP COLUMN IF EXISTS email_verification_code_hash;
ALTER TABLE users DROP COLUMN IF EXISTS email_verified;
