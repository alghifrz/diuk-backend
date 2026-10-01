-- Application permissions and the role_permissions mapping. Roles themselves
-- stay in the Job 02 table; this migration only attaches V1 capabilities.

CREATE TABLE permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        TEXT        NOT NULL UNIQUE,
    resource    TEXT        NOT NULL CHECK (btrim(resource) <> ''),
    action      TEXT        NOT NULL CHECK (btrim(action) <> ''),
    description TEXT        CHECK (description IS NULL OR btrim(description) <> ''),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_permissions_code_format CHECK (code ~ '^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$'),
    CONSTRAINT chk_permissions_code_matches CHECK (code = resource || '.' || action)
);

CREATE TRIGGER trg_permissions_updated_at
    BEFORE UPDATE ON permissions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE role_permissions (
    role_id       UUID        NOT NULL REFERENCES roles (id) ON DELETE RESTRICT,
    permission_id UUID        NOT NULL REFERENCES permissions (id) ON DELETE RESTRICT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_id)
);

-- role_id is the leading column of the primary key. permission_id needs its
-- own index for lookups that start from a permission.
CREATE INDEX idx_role_permissions_permission_id ON role_permissions (permission_id);

INSERT INTO permissions (code, resource, action, description) VALUES
    ('business.read',             'business',        'read',     'Read the current business'),
    ('business.update',           'business',        'update',   'Update the current business'),
    ('business_users.read',       'business_users',  'read',     'List members of the business'),
    ('business_users.create',     'business_users',  'create',   'Add a member to the business'),
    ('business_users.update',     'business_users',  'update',   'Update a business membership'),
    ('business_users.delete',     'business_users',  'delete',   'Remove a business membership'),
    ('area.read',                 'area',            'read',     'Read service areas'),
    ('area.create',               'area',            'create',   'Create a service area'),
    ('area.update',               'area',            'update',   'Update a service area'),
    ('area.delete',               'area',            'delete',   'Delete a service area'),
    ('menu.read',                 'menu',            'read',     'Read the menu'),
    ('menu.create',               'menu',            'create',   'Create a menu item'),
    ('menu.update',               'menu',            'update',   'Update a menu item'),
    ('menu.delete',               'menu',            'delete',   'Delete a menu item'),
    ('customer.read',             'customer',        'read',     'Read customers'),
    ('customer.create',           'customer',        'create',   'Create a customer'),
    ('customer.update',           'customer',        'update',   'Update a customer'),
    ('customer.delete',           'customer',        'delete',   'Delete a customer'),
    ('conversation.read',         'conversation',    'read',     'Read conversations'),
    ('conversation.update',       'conversation',    'update',   'Update a conversation'),
    ('reservation.read',          'reservation',     'read',     'Read reservations'),
    ('reservation.create',        'reservation',     'create',   'Create a reservation'),
    ('reservation.update',        'reservation',     'update',   'Update a reservation'),
    ('reservation.confirm',       'reservation',     'confirm',  'Confirm a reservation'),
    ('reservation.cancel',        'reservation',     'cancel',   'Cancel a reservation'),
    ('prompt.read',               'prompt',          'read',     'Read AI prompts'),
    ('prompt.create',             'prompt',          'create',   'Create an AI prompt'),
    ('prompt.update',             'prompt',          'update',   'Update an AI prompt'),
    ('prompt.delete',             'prompt',          'delete',   'Delete an AI prompt'),
    ('analytics.read',            'analytics',       'read',     'Read analytics'),
    ('report.read',               'report',          'read',     'Read reports')
ON CONFLICT (code) DO NOTHING;

-- OWNER receives every V1 permission through the mapping table. There is no
-- application-level OWNER bypass.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'OWNER'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
    'business.read',
    'business_users.read',
    'area.read', 'area.create', 'area.update', 'area.delete',
    'menu.read', 'menu.create', 'menu.update', 'menu.delete',
    'customer.read', 'customer.create', 'customer.update', 'customer.delete',
    'conversation.read', 'conversation.update',
    'reservation.read', 'reservation.create', 'reservation.update', 'reservation.confirm', 'reservation.cancel',
    'prompt.read', 'prompt.update',
    'analytics.read',
    'report.read'
)
WHERE r.name = 'MANAGER'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
    'business.read',
    'area.read',
    'menu.read',
    'customer.read', 'customer.create', 'customer.update',
    'conversation.read', 'conversation.update',
    'reservation.read', 'reservation.create', 'reservation.update', 'reservation.confirm', 'reservation.cancel'
)
WHERE r.name = 'STAFF'
ON CONFLICT DO NOTHING;
