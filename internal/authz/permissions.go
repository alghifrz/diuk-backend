// Package authz is the database-driven authorization layer. Authentication
// stays in package auth; this package only answers what a tenant role may do.
package authz

// Permission is a resource.action code stored in the permissions table.
type Permission string

// V1 permission codes. Application checks must use these constants rather
// than raw strings.
const (
	PermissionBusinessRead   Permission = "business.read"
	PermissionBusinessUpdate Permission = "business.update"

	PermissionBusinessUsersRead   Permission = "business_users.read"
	PermissionBusinessUsersCreate Permission = "business_users.create"
	PermissionBusinessUsersUpdate Permission = "business_users.update"
	PermissionBusinessUsersDelete Permission = "business_users.delete"

	PermissionAreaRead   Permission = "area.read"
	PermissionAreaCreate Permission = "area.create"
	PermissionAreaUpdate Permission = "area.update"
	PermissionAreaDelete Permission = "area.delete"

	PermissionMenuRead   Permission = "menu.read"
	PermissionMenuCreate Permission = "menu.create"
	PermissionMenuUpdate Permission = "menu.update"
	PermissionMenuDelete Permission = "menu.delete"

	PermissionCustomerRead   Permission = "customer.read"
	PermissionCustomerCreate Permission = "customer.create"
	PermissionCustomerUpdate Permission = "customer.update"
	PermissionCustomerDelete Permission = "customer.delete"

	PermissionConversationRead   Permission = "conversation.read"
	PermissionConversationUpdate Permission = "conversation.update"

	PermissionReservationRead    Permission = "reservation.read"
	PermissionReservationCreate  Permission = "reservation.create"
	PermissionReservationUpdate  Permission = "reservation.update"
	PermissionReservationConfirm Permission = "reservation.confirm"
	PermissionReservationCancel  Permission = "reservation.cancel"

	PermissionPromptRead   Permission = "prompt.read"
	PermissionPromptCreate Permission = "prompt.create"
	PermissionPromptUpdate Permission = "prompt.update"
	PermissionPromptDelete Permission = "prompt.delete"

	PermissionChannelRead   Permission = "channel.read"
	PermissionChannelCreate Permission = "channel.create"
	PermissionChannelUpdate Permission = "channel.update"
	PermissionChannelDelete Permission = "channel.delete"

	PermissionKnowledgeRead   Permission = "knowledge.read"
	PermissionKnowledgeCreate Permission = "knowledge.create"
	PermissionKnowledgeUpdate Permission = "knowledge.update"
	PermissionKnowledgeDelete Permission = "knowledge.delete"

	PermissionAutomationRead   Permission = "automation.read"
	PermissionAutomationCreate Permission = "automation.create"
	PermissionAutomationUpdate Permission = "automation.update"
	PermissionAutomationDelete Permission = "automation.delete"

	PermissionAnalyticsRead Permission = "analytics.read"
	PermissionReportRead    Permission = "report.read"

	PermissionCampaignRead   Permission = "campaign.read"
	PermissionCampaignCreate Permission = "campaign.create"
	PermissionCampaignUpdate Permission = "campaign.update"
	PermissionCampaignDelete Permission = "campaign.delete"

	PermissionUsageRead Permission = "usage.read"

	PermissionPaymentRead   Permission = "payment.read"
	PermissionPaymentUpdate Permission = "payment.update"

	PermissionCalendarRead   Permission = "calendar.read"
	PermissionCalendarUpdate Permission = "calendar.update"
)

// All returns every V1 permission. OWNER is mapped to this full set in the
// database; the application never special-cases that role.
func All() []Permission {
	return []Permission{
		PermissionBusinessRead,
		PermissionBusinessUpdate,
		PermissionBusinessUsersRead,
		PermissionBusinessUsersCreate,
		PermissionBusinessUsersUpdate,
		PermissionBusinessUsersDelete,
		PermissionAreaRead,
		PermissionAreaCreate,
		PermissionAreaUpdate,
		PermissionAreaDelete,
		PermissionMenuRead,
		PermissionMenuCreate,
		PermissionMenuUpdate,
		PermissionMenuDelete,
		PermissionCustomerRead,
		PermissionCustomerCreate,
		PermissionCustomerUpdate,
		PermissionCustomerDelete,
		PermissionConversationRead,
		PermissionConversationUpdate,
		PermissionReservationRead,
		PermissionReservationCreate,
		PermissionReservationUpdate,
		PermissionReservationConfirm,
		PermissionReservationCancel,
		PermissionPromptRead,
		PermissionPromptCreate,
		PermissionPromptUpdate,
		PermissionPromptDelete,
		PermissionChannelRead,
		PermissionChannelCreate,
		PermissionChannelUpdate,
		PermissionChannelDelete,
		PermissionKnowledgeRead,
		PermissionKnowledgeCreate,
		PermissionKnowledgeUpdate,
		PermissionKnowledgeDelete,
		PermissionAutomationRead,
		PermissionAutomationCreate,
		PermissionAutomationUpdate,
		PermissionAutomationDelete,
		PermissionAnalyticsRead,
		PermissionReportRead,
		PermissionCampaignRead,
		PermissionCampaignCreate,
		PermissionCampaignUpdate,
		PermissionCampaignDelete,
		PermissionUsageRead,
		PermissionPaymentRead,
		PermissionPaymentUpdate,
		PermissionCalendarRead,
		PermissionCalendarUpdate,
	}
}

// ManagerPermissions is the V1 set seeded for MANAGER.
func ManagerPermissions() []Permission {
	return []Permission{
		PermissionBusinessRead,
		PermissionBusinessUsersRead,
		PermissionAreaRead,
		PermissionAreaCreate,
		PermissionAreaUpdate,
		PermissionAreaDelete,
		PermissionMenuRead,
		PermissionMenuCreate,
		PermissionMenuUpdate,
		PermissionMenuDelete,
		PermissionCustomerRead,
		PermissionCustomerCreate,
		PermissionCustomerUpdate,
		PermissionCustomerDelete,
		PermissionConversationRead,
		PermissionConversationUpdate,
		PermissionReservationRead,
		PermissionReservationCreate,
		PermissionReservationUpdate,
		PermissionReservationConfirm,
		PermissionReservationCancel,
		PermissionPromptRead,
		PermissionPromptUpdate,
		PermissionChannelRead,
		PermissionChannelCreate,
		PermissionChannelUpdate,
		PermissionChannelDelete,
		PermissionKnowledgeRead,
		PermissionKnowledgeCreate,
		PermissionKnowledgeUpdate,
		PermissionKnowledgeDelete,
		PermissionAutomationRead,
		PermissionAutomationCreate,
		PermissionAutomationUpdate,
		PermissionAutomationDelete,
		PermissionAnalyticsRead,
		PermissionReportRead,
		PermissionCampaignRead,
		PermissionCampaignCreate,
		PermissionCampaignUpdate,
		PermissionCampaignDelete,
		PermissionUsageRead,
		PermissionPaymentRead,
		PermissionPaymentUpdate,
		PermissionCalendarRead,
		PermissionCalendarUpdate,
	}
}

// StaffPermissions is the V1 set seeded for STAFF.
func StaffPermissions() []Permission {
	return []Permission{
		PermissionBusinessRead,
		PermissionAreaRead,
		PermissionMenuRead,
		PermissionCustomerRead,
		PermissionCustomerCreate,
		PermissionCustomerUpdate,
		PermissionConversationRead,
		PermissionConversationUpdate,
		PermissionReservationRead,
		PermissionReservationCreate,
		PermissionReservationUpdate,
		PermissionReservationConfirm,
		PermissionReservationCancel,
		PermissionKnowledgeRead,
		PermissionAutomationRead,
		PermissionCampaignRead,
		PermissionUsageRead,
		PermissionPaymentRead,
		PermissionCalendarRead,
	}
}

func permissionSet(list []Permission) map[Permission]struct{} {
	set := make(map[Permission]struct{}, len(list))
	for _, p := range list {
		set[p] = struct{}{}
	}
	return set
}
