-- Channel configuration for messaging providers. Secrets stay in the
-- environment; this table only stores tenant-visible account identifiers.

CREATE TABLE business_channels (
    id                        UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id               UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    channel                   VARCHAR(20)  NOT NULL CHECK (channel IN ('WHATSAPP', 'INSTAGRAM')),
    status                    VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    display_name              VARCHAR(100) CHECK (display_name IS NULL OR btrim(display_name) <> ''),
    external_account_id       VARCHAR(128) CHECK (external_account_id IS NULL OR btrim(external_account_id) <> ''),
    external_phone_number_id  VARCHAR(128) CHECK (external_phone_number_id IS NULL OR btrim(external_phone_number_id) <> ''),
    created_at                TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_business_channels_business_channel_account
        UNIQUE (business_id, channel, external_account_id)
);

CREATE UNIQUE INDEX uq_business_channels_phone_number
    ON business_channels (channel, external_phone_number_id)
    WHERE external_phone_number_id IS NOT NULL AND status = 'ACTIVE';

CREATE INDEX idx_business_channels_business_id_channel
    ON business_channels (business_id, channel, status);

CREATE TRIGGER trg_business_channels_updated_at
    BEFORE UPDATE ON business_channels
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Provider message IDs are unique inside a tenant so status webhooks can
-- resolve outbound messages without a conversation id.
CREATE UNIQUE INDEX uq_messages_business_external_id
    ON messages (business_id, external_id)
    WHERE external_id IS NOT NULL;

INSERT INTO permissions (code, resource, action, description) VALUES
    ('channel.read',   'channel', 'read',   'Read messaging channel configuration'),
    ('channel.create', 'channel', 'create', 'Create messaging channel configuration'),
    ('channel.update', 'channel', 'update', 'Update messaging channel configuration'),
    ('channel.delete', 'channel', 'delete', 'Deactivate messaging channel configuration')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'OWNER' AND p.code LIKE 'channel.%'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('channel.read', 'channel.create', 'channel.update', 'channel.delete')
WHERE r.name = 'MANAGER'
ON CONFLICT DO NOTHING;
