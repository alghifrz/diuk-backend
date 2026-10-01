-- Adjust AI Response tiers: STANDARD 25k, PLUS 50k, PRO 150k.
-- Existing STANDARD businesses move to PLUS so they keep the previous 50k quota.

ALTER TABLE plans DROP CONSTRAINT IF EXISTS plans_code_check;
ALTER TABLE plans ADD CONSTRAINT plans_code_check
    CHECK (code IN ('FREE', 'STANDARD', 'PLUS', 'PRO', 'BUSINESS'));

INSERT INTO plans (id, code, name, description, status) VALUES
    ('00000000-0000-4000-8000-000000000005', 'PLUS', 'Plus', 'Mid-tier plan with 50,000 AI Responses per month', 'ACTIVE')
ON CONFLICT (id) DO NOTHING;

INSERT INTO plan_entitlements (plan_id, feature_key, limit_value, period, enabled) VALUES
    ('00000000-0000-4000-8000-000000000005', 'AI_RESPONSES',        50000,  'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000005', 'CAMPAIGN_RECIPIENTS', 100000, 'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000005', 'AUTOMATION_RUNS',     10000,  'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000005', 'KNOWLEDGE_DOCUMENTS', 100,    'NONE',    TRUE),
    ('00000000-0000-4000-8000-000000000005', 'WHATSAPP_CHANNELS',   3,      'NONE',    TRUE)
ON CONFLICT (plan_id, feature_key) DO NOTHING;

UPDATE plan_entitlements
SET limit_value = 25000
WHERE plan_id = '00000000-0000-4000-8000-000000000002'
  AND feature_key = 'AI_RESPONSES';

UPDATE plans
SET description = 'Entry plan with 25,000 AI Responses per month'
WHERE id = '00000000-0000-4000-8000-000000000002';

UPDATE businesses
SET plan_id = '00000000-0000-4000-8000-000000000005'
WHERE plan_id = '00000000-0000-4000-8000-000000000002';

ALTER TABLE businesses
    ALTER COLUMN plan_id SET DEFAULT '00000000-0000-4000-8000-000000000005';
