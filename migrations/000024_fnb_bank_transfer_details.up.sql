-- JOB 24: bank transfer details for AI payment instructions.
ALTER TABLE fnb_settings
    ADD COLUMN IF NOT EXISTS bank_name            TEXT CHECK (bank_name IS NULL OR btrim(bank_name) <> ''),
    ADD COLUMN IF NOT EXISTS bank_account_number  TEXT CHECK (bank_account_number IS NULL OR btrim(bank_account_number) <> ''),
    ADD COLUMN IF NOT EXISTS bank_account_holder  TEXT CHECK (bank_account_holder IS NULL OR btrim(bank_account_holder) <> '');
