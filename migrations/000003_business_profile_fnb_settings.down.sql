DROP TRIGGER IF EXISTS trg_businesses_default_fnb_settings ON businesses;
DROP FUNCTION IF EXISTS create_default_fnb_settings();

DROP TRIGGER IF EXISTS trg_fnb_settings_business_type ON fnb_settings;
DROP FUNCTION IF EXISTS enforce_fnb_settings_business_type();

DROP TABLE IF EXISTS fnb_settings;

ALTER TABLE businesses
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS phone,
    DROP COLUMN IF EXISTS email,
    DROP COLUMN IF EXISTS address,
    DROP COLUMN IF EXISTS website_url,
    DROP COLUMN IF EXISTS logo_url;
