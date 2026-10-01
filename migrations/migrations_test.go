package migrations

import (
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// TestSourceIsUsable proves golang-migrate can read the embedded migrations
// and that every version has both an up and a down file.
func TestSourceIsUsable(t *testing.T) {
	driver, err := iofs.New(FS, ".")
	if err != nil {
		t.Fatalf("iofs.New() error = %v", err)
	}
	defer driver.Close()

	version, err := driver.First()
	if err != nil {
		t.Fatalf("First() error = %v", err)
	}

	for {
		for _, direction := range []string{"up", "down"} {
			readMigration(t, driver, version, direction)
		}

		next, err := driver.Next(version)
		if err != nil {
			break
		}
		version = next
	}

	if version < 18 {
		t.Fatalf("last version = %d, want at least 18", version)
	}
}

func TestCampaignMigration(t *testing.T) {
	up := readFile(t, "000016_create_campaign_tables.up.sql")
	down := readFile(t, "000016_create_campaign_tables.down.sql")
	for _, fragment := range []string{
		"CREATE TABLE campaigns",
		"CREATE TABLE campaign_runs",
		"CREATE TABLE campaign_recipients",
		"marketing_opt_in",
		"CAMPAIGN_START",
		"CAMPAIGN_DISPATCH",
		"CAMPAIGN",
		"campaign.read",
		"campaign.create",
		"uq_campaign_runs_one_active",
		"uq_campaign_recipients_run_customer",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS campaign_recipients") ||
		!strings.Contains(down, "DROP TABLE IF EXISTS campaign_runs") ||
		!strings.Contains(down, "DROP TABLE IF EXISTS campaigns") {
		t.Error("down migration must drop campaign tables")
	}
	if strings.Index(down, "DROP TABLE IF EXISTS campaign_recipients") > strings.Index(down, "DROP TABLE IF EXISTS campaigns") {
		t.Error("down migration must drop recipients before campaigns")
	}
	if !strings.Contains(down, "DELETE FROM permissions WHERE code LIKE 'campaign.%'") {
		t.Error("down migration must remove campaign permissions")
	}
	if strings.Contains(down, "DROP TABLE IF EXISTS messages") || strings.Contains(down, "DROP TABLE IF EXISTS customers") {
		t.Error("down migration must not drop JOB 01-17 tables")
	}
}

func TestUsageMigration(t *testing.T) {
	up := readFile(t, "000017_create_usage_tables.up.sql")
	down := readFile(t, "000017_create_usage_tables.down.sql")
	for _, fragment := range []string{
		"CREATE TABLE plans",
		"CREATE TABLE plan_entitlements",
		"CREATE TABLE usage_periods",
		"CREATE TABLE usage_counters",
		"CREATE TABLE usage_consumptions",
		"plan_id",
		"usage.read",
		"AI_RESPONSES",
		"CAMPAIGN_RECIPIENTS",
		"AUTOMATION_RUNS",
		"KNOWLEDGE_DOCUMENTS",
		"WHATSAPP_CHANNELS",
		"00000000-0000-4000-8000-000000000002",
		"50000",
		"uq_usage_periods_business_type_start",
		"uq_usage_consumptions_source",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS usage_consumptions") ||
		!strings.Contains(down, "DROP TABLE IF EXISTS usage_counters") ||
		!strings.Contains(down, "DROP TABLE IF EXISTS usage_periods") ||
		!strings.Contains(down, "DROP TABLE IF EXISTS plan_entitlements") ||
		!strings.Contains(down, "DROP TABLE IF EXISTS plans") {
		t.Error("down migration must drop usage and plan tables")
	}
	if !strings.Contains(down, "ALTER TABLE businesses DROP COLUMN IF EXISTS plan_id") {
		t.Error("down migration must drop businesses.plan_id")
	}
	if !strings.Contains(down, "DELETE FROM permissions WHERE code = 'usage.read'") {
		t.Error("down migration must remove usage.read")
	}
	if strings.Contains(down, "DROP TABLE IF EXISTS messages") || strings.Contains(down, "DROP TABLE IF EXISTS businesses") {
		t.Error("down migration must not drop JOB 01-18 tables")
	}
}

func TestPlusPlanMigration(t *testing.T) {
	up := readFile(t, "000018_add_plus_plan.up.sql")
	down := readFile(t, "000018_add_plus_plan.down.sql")
	for _, fragment := range []string{
		"PLUS",
		"25000",
		"50000",
		"00000000-0000-4000-8000-000000000005",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if !strings.Contains(down, "DELETE FROM plans") || !strings.Contains(down, "00000000-0000-4000-8000-000000000005") {
		t.Error("down migration must remove the PLUS plan")
	}
}

func TestPaymentMigration(t *testing.T) {
	up := readFile(t, "000019_create_manual_payment_tables.up.sql")
	down := readFile(t, "000019_create_manual_payment_tables.down.sql")
	for _, fragment := range []string{
		"reservation_payments",
		"deposit_enabled",
		"deposit_type",
		"deposit_value",
		"payment.read",
		"payment.update",
		"UNIQUE (business_id, reservation_id)",
		"fk_reservation_payments_reservation_business",
		"UNPAID",
		"PAID",
		"REFUNDED",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS reservation_payments") {
		t.Error("down migration must drop reservation_payments")
	}
	if !strings.Contains(down, "DELETE FROM permissions WHERE code LIKE 'payment.%'") {
		t.Error("down migration must remove payment permissions")
	}
	if strings.Contains(down, "DROP TABLE IF EXISTS reservations") || strings.Contains(down, "DROP TABLE IF EXISTS businesses") {
		t.Error("down migration must not drop earlier tables")
	}
}

func TestCalendarMigration(t *testing.T) {
	up := readFile(t, "000020_create_calendar_tables.up.sql")
	down := readFile(t, "000020_create_calendar_tables.down.sql")
	for _, fragment := range []string{
		"calendar_blocks",
		"calendar.read",
		"calendar.update",
		"chk_calendar_blocks_scope",
		"excl_calendar_blocks_business_overlap",
		"excl_calendar_blocks_area_overlap",
		"excl_calendar_blocks_table_overlap",
		"ACTIVE",
		"CANCELLED",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS calendar_blocks") {
		t.Error("down migration must drop calendar_blocks")
	}
	if !strings.Contains(down, "DELETE FROM permissions WHERE code LIKE 'calendar.%'") {
		t.Error("down migration must remove calendar permissions")
	}
	if strings.Contains(down, "DROP TABLE IF EXISTS reservations") || strings.Contains(down, "DROP TABLE IF EXISTS businesses") {
		t.Error("down migration must not drop earlier tables")
	}
}

func TestRevenueConversionMigration(t *testing.T) {
	up := readFile(t, "000021_revenue_conversion_analytics.up.sql")
	down := readFile(t, "000021_revenue_conversion_analytics.down.sql")
	for _, fragment := range []string{
		"idx_reservations_business_id_created_at_status",
		"idx_reservation_payments_business_id_created_at_status",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if strings.Contains(up, "CREATE TABLE analytics_events") {
		t.Error("must not create analytics_events")
	}
	if !strings.Contains(down, "DROP INDEX IF EXISTS idx_reservations_business_id_created_at_status") {
		t.Error("down migration must drop reservation analytics index")
	}
	if !strings.Contains(down, "DROP INDEX IF EXISTS idx_reservation_payments_business_id_created_at_status") {
		t.Error("down migration must drop payment analytics index")
	}
	if strings.Contains(down, "DROP TABLE IF EXISTS reservations") || strings.Contains(down, "DROP TABLE IF EXISTS reservation_payments") {
		t.Error("down migration must not drop earlier tables")
	}
}

func TestAnalyticsMigration(t *testing.T) {
	up := readFile(t, "000015_create_analytics_tables.up.sql")
	down := readFile(t, "000015_create_analytics_tables.down.sql")
	for _, fragment := range []string{
		"ALTER TABLE ai_generations",
		"started_at",
		"completed_at",
		"latency_ms",
		"input_tokens",
		"output_tokens",
		"total_tokens",
		"idx_customers_business_id_created_at",
		"idx_conversations_business_id_created_at",
		"idx_messages_business_id_created_at_customer_facing",
		"idx_ai_generations_business_id_created_at",
		"idx_automation_runs_business_id_created_at",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if strings.Contains(up, "CREATE TABLE analytics_events") {
		t.Error("must not create analytics_events")
	}
	for _, leftover := range []string{"DROP TABLE IF EXISTS conversations", "DROP TABLE IF EXISTS messages", "DROP TABLE IF EXISTS ai_generations"} {
		if strings.Contains(down, leftover) {
			t.Errorf("down migration must not %s", leftover)
		}
	}
}

func TestHumanHandoverMigration(t *testing.T) {
	up := readFile(t, "000014_create_human_handover_tables.up.sql")
	down := readFile(t, "000014_create_human_handover_tables.down.sql")
	for _, fragment := range []string{
		"handling_state",
		"handover_reason",
		"handover_requested_at",
		"human_started_at",
		"resolved_at",
		"AI_ACTIVE",
		"HUMAN_REQUESTED",
		"HUMAN_ACTIVE",
		"RESOLVED",
		"CUSTOMER_REQUESTED_HUMAN",
		"AI_UNABLE_TO_ANSWER",
		"MANUAL_HANDOFF",
		"INTERNAL_NOTE",
		"INTERNAL",
		"idx_conversations_business_id_handling_state",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if !strings.Contains(down, "DROP INDEX IF EXISTS idx_conversations_business_id_handling_state") {
		t.Error("down migration must drop handling_state index")
	}
	if !strings.Contains(down, "DROP COLUMN IF EXISTS handling_state") {
		t.Error("down migration must drop handling columns")
	}
	if strings.Contains(down, "DROP TABLE IF EXISTS conversations") || strings.Contains(down, "DROP TABLE IF EXISTS messages") {
		t.Error("down migration must not drop conversations or messages")
	}
}

func TestAutomationMigration(t *testing.T) {
	up := readFile(t, "000013_create_automation_tables.up.sql")
	down := readFile(t, "000013_create_automation_tables.down.sql")
	for _, fragment := range []string{
		"CREATE TABLE automations",
		"CREATE TABLE automation_events",
		"CREATE TABLE automation_runs",
		"uq_automation_events_idempotency",
		"uq_automation_runs_event",
		"uq_automation_runs_execution_key",
		"AUTOMATION_EVALUATE",
		"AUTOMATION_RUN",
		"automation.read",
		"automation.create",
		"automation.update",
		"automation.delete",
		"set_updated_at()",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS automation_runs") ||
		!strings.Contains(down, "DROP TABLE IF EXISTS automation_events") ||
		!strings.Contains(down, "DROP TABLE IF EXISTS automations") {
		t.Error("down migration must drop automation tables")
	}
	if strings.Index(down, "DROP TABLE IF EXISTS automation_runs") > strings.Index(down, "DROP TABLE IF EXISTS automations") {
		t.Error("down migration must drop runs before automations")
	}
	if !strings.Contains(down, "DELETE FROM permissions WHERE code LIKE 'automation.%'") {
		t.Error("down migration must remove automation permissions")
	}
	if strings.Contains(down, "DROP TABLE IF EXISTS job_outbox") || strings.Contains(down, "DROP TABLE IF EXISTS businesses") {
		t.Error("down migration must not drop JOB 01-14 tables")
	}
}

func TestKnowledgeMigration(t *testing.T) {
	up := readFile(t, "000012_create_knowledge_tables.up.sql")
	down := readFile(t, "000012_create_knowledge_tables.down.sql")
	for _, fragment := range []string{
		"CREATE EXTENSION IF NOT EXISTS vector",
		"CREATE TABLE knowledge_documents",
		"CREATE TABLE knowledge_chunks",
		"VECTOR(1536)",
		"vector_cosine_ops",
		"KNOWLEDGE_INGEST",
		"knowledge.read",
		"knowledge.create",
		"knowledge.update",
		"knowledge.delete",
		"ON DELETE CASCADE",
		"set_updated_at()",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS knowledge_chunks") || !strings.Contains(down, "DROP TABLE IF EXISTS knowledge_documents") {
		t.Error("down migration must drop knowledge tables")
	}
	if strings.Index(down, "DROP TABLE IF EXISTS knowledge_chunks") > strings.Index(down, "DROP TABLE IF EXISTS knowledge_documents") {
		t.Error("down migration must drop chunks before documents")
	}
	if !strings.Contains(down, "DELETE FROM permissions WHERE code LIKE 'knowledge.%'") {
		t.Error("down migration must remove knowledge permissions")
	}
	if strings.Contains(down, "DROP TABLE IF EXISTS job_outbox") || strings.Contains(down, "DROP TABLE IF EXISTS businesses") {
		t.Error("down migration must not drop JOB 01-13 tables")
	}
}

func TestJobMigration(t *testing.T) {
	up := readFile(t, "000011_create_job_tables.up.sql")
	down := readFile(t, "000011_create_job_tables.down.sql")
	for _, fragment := range []string{"CREATE TABLE job_outbox", "CREATE TABLE ai_generations", "SENDING", "AI_GENERATE"} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS job_outbox") || !strings.Contains(down, "DROP TABLE IF EXISTS ai_generations") {
		t.Error("down migration must drop job tables")
	}
	if strings.Contains(down, "DROP TABLE IF EXISTS messages") {
		t.Error("down migration must not drop messages")
	}
}

func TestChannelMigration(t *testing.T) {
	up := readFile(t, "000010_create_channel_tables.up.sql")
	down := readFile(t, "000010_create_channel_tables.down.sql")

	if !strings.Contains(up, "CREATE TABLE business_channels") {
		t.Error("up migration does not create business_channels")
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS business_channels") {
		t.Error("down migration must drop business_channels")
	}
	for _, fragment := range []string{
		"uq_business_channels_business_channel_account",
		"uq_business_channels_phone_number",
		"uq_messages_business_external_id",
		"channel.read",
		"channel.create",
		"WHATSAPP",
		"INSTAGRAM",
		"set_updated_at()",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if !strings.Contains(down, "DELETE FROM permissions WHERE code LIKE 'channel.%'") {
		t.Error("down migration must remove channel permissions")
	}
	if strings.Contains(down, "DROP TABLE IF EXISTS messages") || strings.Contains(down, "DROP TABLE IF EXISTS businesses") {
		t.Error("down migration must not drop JOB 01-09 tables")
	}
}

func TestAIMigration(t *testing.T) {
	up := readFile(t, "000009_create_ai_tables.up.sql")
	down := readFile(t, "000009_create_ai_tables.down.sql")

	for _, table := range []string{"ai_settings", "ai_prompts"} {
		if !strings.Contains(up, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create table %q", table)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS ai_prompts") || !strings.Contains(down, "DROP TABLE IF EXISTS ai_settings") {
		t.Error("down migration must drop ai_prompts then ai_settings")
	}
	if strings.Index(down, "DROP TABLE IF EXISTS ai_prompts") > strings.Index(down, "DROP TABLE IF EXISTS ai_settings") {
		t.Error("down migration must drop ai_prompts before ai_settings")
	}
	for _, fragment := range []string{
		"uq_ai_prompts_business_name_version",
		"uq_ai_prompts_one_active",
		"CHECK (provider IN",
		"temperature >= 0",
		"max_output_tokens > 0",
		"status IN ('DRAFT', 'ACTIVE', 'ARCHIVED')",
		"version >= 1",
		"set_updated_at()",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"api_key", "api key", "secret"} {
		if strings.Contains(strings.ToLower(up), forbidden) {
			t.Errorf("up migration must not store %q", forbidden)
		}
	}
	for _, leftover := range []string{"reservations", "conversations", "messages", "businesses"} {
		if leftover != "businesses" && strings.Contains(down, leftover) {
			t.Errorf("down migration must not touch %q", leftover)
		}
	}
}

func TestReservationMigration(t *testing.T) {
	up := readFile(t, "000008_create_reservation_tables.up.sql")
	down := readFile(t, "000008_create_reservation_tables.down.sql")

	for _, table := range []string{"business_hours", "business_schedule_exceptions", "reservations"} {
		if !strings.Contains(up, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create table %q", table)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS reservations") ||
		!strings.Contains(down, "DROP TABLE IF EXISTS business_schedule_exceptions") ||
		!strings.Contains(down, "DROP TABLE IF EXISTS business_hours") {
		t.Error("down migration must drop reservation objects")
	}
	if strings.Index(down, "DROP TABLE IF EXISTS reservations") > strings.Index(down, "DROP TABLE IF EXISTS business_hours") {
		t.Error("down migration must drop reservations before hours")
	}
	for _, fragment := range []string{
		"btree_gist",
		"excl_reservations_table_overlap",
		"fk_reservations_customer_business",
		"idx_reservations_business_id_start_at",
		"uq_business_hours_business_day_sort",
		"set_updated_at()",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
}

func TestConversationMigration(t *testing.T) {
	up := readFile(t, "000007_create_conversation_tables.up.sql")
	down := readFile(t, "000007_create_conversation_tables.down.sql")

	for _, table := range []string{"conversations", "messages"} {
		if !strings.Contains(up, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create table %q", table)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS messages") || !strings.Contains(down, "DROP TABLE IF EXISTS conversations") {
		t.Error("down migration must drop messages then conversations")
	}
	if strings.Index(down, "DROP TABLE IF EXISTS messages") > strings.Index(down, "DROP TABLE IF EXISTS conversations") {
		t.Error("down migration must drop messages before conversations")
	}
	for _, fragment := range []string{
		"uq_customer_identities_id_business_customer",
		"fk_conversations_customer_business",
		"fk_conversations_identity_customer_business",
		"fk_messages_conversation_business",
		"uq_conversations_open_with_identity",
		"uq_conversations_open_without_identity",
		"uq_messages_conversation_external_id",
		"idx_conversations_business_id_status_last_message_at",
		"WHATSAPP",
		"set_updated_at()",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
}

func TestPermissionsMigrationCreatesTables(t *testing.T) {
	up := readFile(t, "000002_create_permissions.up.sql")
	down := readFile(t, "000002_create_permissions.down.sql")

	for _, table := range []string{"permissions", "role_permissions"} {
		if !strings.Contains(up, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create table %q", table)
		}
		if !strings.Contains(down, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("down migration does not drop table %q", table)
		}
	}

	for _, fragment := range []string{"PRIMARY KEY (role_id, permission_id)", "business.read", "reservation.confirm", "WHERE r.name = 'OWNER'", "WHERE r.name = 'MANAGER'", "WHERE r.name = 'STAFF'"} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if strings.Contains(up, "reservation.delete") {
		t.Error("up migration must not seed reservation.delete")
	}
}

func TestBusinessProfileMigration(t *testing.T) {
	up := readFile(t, "000003_business_profile_fnb_settings.up.sql")
	down := readFile(t, "000003_business_profile_fnb_settings.down.sql")

	for _, fragment := range []string{"ADD COLUMN description", "CREATE TABLE fnb_settings", "PRIMARY KEY", "currency", "reservation_slot_duration_minutes"} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS fnb_settings") {
		t.Error("down migration does not drop fnb_settings")
	}
}

func TestCustomerCRMMigration(t *testing.T) {
	up := readFile(t, "000006_create_customer_crm_tables.up.sql")
	down := readFile(t, "000006_create_customer_crm_tables.down.sql")

	for _, table := range []string{"customers", "customer_identities", "tags", "customer_tags"} {
		if !strings.Contains(up, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create table %q", table)
		}
	}
	for _, table := range []string{"customer_tags", "customer_identities", "tags", "customers"} {
		if !strings.Contains(down, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("down migration does not drop %q", table)
		}
	}
	for _, fragment := range []string{
		"UNIQUE (business_id, channel, external_id)",
		"UNIQUE (business_id, name)",
		"fk_customer_identities_customer_business",
		"fk_customer_tags_customer_business",
		"fk_customer_tags_tag_business",
		"WHATSAPP",
		"set_updated_at()",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
}

func TestMenuTablesMigration(t *testing.T) {
	up := readFile(t, "000005_create_menu_tables.up.sql")
	down := readFile(t, "000005_create_menu_tables.down.sql")

	for _, table := range []string{"menu_categories", "menu_items"} {
		if !strings.Contains(up, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create table %q", table)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS menu_items") || !strings.Contains(down, "DROP TABLE IF EXISTS menu_categories") {
		t.Error("down migration must drop menu_items then menu_categories")
	}
	if strings.Index(down, "DROP TABLE IF EXISTS menu_items") > strings.Index(down, "DROP TABLE IF EXISTS menu_categories") {
		t.Error("down migration must drop menu_items before menu_categories")
	}
	for _, fragment := range []string{
		"UNIQUE (business_id, name)",
		"NUMERIC(15,2)",
		"idx_menu_categories_business_id_status",
		"idx_menu_items_category_id_status",
		"set_updated_at()",
		"price >= 0",
		"sort_order >= 0",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
}

func TestAreasAndTablesMigration(t *testing.T) {
	up := readFile(t, "000004_create_areas_and_tables.up.sql")
	down := readFile(t, "000004_create_areas_and_tables.down.sql")

	for _, table := range []string{"areas", "tables"} {
		if !strings.Contains(up, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create table %q", table)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS tables") || !strings.Contains(down, "DROP TABLE IF EXISTS areas") {
		t.Error("down migration must drop tables then areas")
	}
	if strings.Index(down, "DROP TABLE IF EXISTS tables") > strings.Index(down, "DROP TABLE IF EXISTS areas") {
		t.Error("down migration must drop tables before areas")
	}

	for _, fragment := range []string{
		"UNIQUE (business_id, name)",
		"UNIQUE (area_id, name)",
		"REFERENCES businesses (id)",
		"REFERENCES areas (id)",
		"idx_areas_business_id_status",
		"idx_tables_area_id_status",
		"set_updated_at()",
		"capacity >= 1",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}
}

func TestFirstMigrationCreatesCoreTables(t *testing.T) {
	up := readFile(t, "000001_create_core_tables.up.sql")
	down := readFile(t, "000001_create_core_tables.down.sql")

	for _, table := range []string{"businesses", "users", "roles", "business_users"} {
		if !strings.Contains(up, "CREATE TABLE "+table) {
			t.Errorf("up migration does not create table %q", table)
		}
		if !strings.Contains(down, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("down migration does not drop table %q", table)
		}
	}

	for _, fragment := range []string{"gen_random_uuid()", "TIMESTAMPTZ", "REFERENCES businesses (id)", "UNIQUE (business_id, user_id)"} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing %q", fragment)
		}
	}

	// Credentials belong to Supabase Auth and must never reach this schema.
	for _, forbidden := range []string{"password", "refresh_token", "access_token"} {
		if strings.Contains(strings.ToLower(up), forbidden) {
			t.Errorf("up migration must not define a %q column", forbidden)
		}
	}
}

func readMigration(t *testing.T, driver source.Driver, version uint, direction string) {
	t.Helper()

	var (
		body io.ReadCloser
		err  error
	)
	if direction == "up" {
		body, _, err = driver.ReadUp(version)
	} else {
		body, _, err = driver.ReadDown(version)
	}
	if err != nil {
		t.Fatalf("read %s migration %d: %v", direction, version, err)
	}
	defer body.Close()

	content, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read %s migration %d body: %v", direction, version, err)
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		t.Errorf("%s migration %d is empty", direction, version)
	}
}

func readFile(t *testing.T, name string) string {
	t.Helper()

	content, err := fs.ReadFile(FS, name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	return string(content)
}
