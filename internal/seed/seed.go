// Package seed inserts the Kasiko Coffee development tenant used to exercise
// real JWT authentication. It never talks to Supabase Auth and never writes
// passwords or tokens.
package seed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

// Development values the seeder is allowed to create or reuse.
const (
	BusinessName     = "Kasiko Coffee"
	BusinessSlug     = "kasiko-coffee"
	BusinessTimezone = "Asia/Jakarta"
	UserFullName     = "Admin Kasiko"
)

// Input is the development Auth identity supplied through the environment.
type Input struct {
	AuthUserID uuid.UUID
	Email      string
}

// Result is the set of rows the seeder resolved.
type Result struct {
	Business   *tenant.Business
	User       *tenant.User
	Role       *tenant.Role
	Membership *tenant.BusinessUser
}

// LoadInput reads SEED_AUTH_USER_ID and SEED_USER_EMAIL. It fails before any
// database work when a value is missing or the Auth user id is not a UUID.
func LoadInput() (Input, error) {
	rawID := strings.TrimSpace(os.Getenv("SEED_AUTH_USER_ID"))
	email := strings.TrimSpace(os.Getenv("SEED_USER_EMAIL"))

	var missing []string
	if rawID == "" {
		missing = append(missing, "SEED_AUTH_USER_ID")
	}
	if email == "" {
		missing = append(missing, "SEED_USER_EMAIL")
	}
	if len(missing) > 0 {
		return Input{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	authUserID, err := uuid.Parse(rawID)
	if err != nil {
		return Input{}, fmt.Errorf("SEED_AUTH_USER_ID must be a valid UUID")
	}

	return Input{AuthUserID: authUserID, Email: tenant.NormalizeEmail(email)}, nil
}

// Run finds or creates the Kasiko Coffee development records using q, which
// should be a transaction so a failure leaves no partial seed behind.
func Run(ctx context.Context, q tenant.Querier, in Input) (Result, error) {
	if in.AuthUserID == uuid.Nil {
		return Result{}, fmt.Errorf("SEED_AUTH_USER_ID must be a valid UUID")
	}
	if strings.TrimSpace(in.Email) == "" {
		return Result{}, fmt.Errorf("missing required environment variables: SEED_USER_EMAIL")
	}

	role, err := tenant.NewRoleRepository(q).GetByName(ctx, tenant.RoleOwner)
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) {
			return Result{}, fmt.Errorf("OWNER role is missing; apply the core migrations before seeding")
		}
		return Result{}, fmt.Errorf("lookup OWNER role: %w", err)
	}

	business, err := ensureBusiness(ctx, tenant.NewBusinessRepository(q))
	if err != nil {
		return Result{}, err
	}

	user, err := ensureUser(ctx, tenant.NewUserRepository(q), in)
	if err != nil {
		return Result{}, err
	}

	membership, err := ensureMembership(ctx, tenant.NewBusinessUserRepository(q), business.ID, user.ID, role.ID)
	if err != nil {
		return Result{}, err
	}

	return Result{Business: business, User: user, Role: role, Membership: membership}, nil
}

func ensureBusiness(ctx context.Context, repo *tenant.BusinessRepository) (*tenant.Business, error) {
	business, err := repo.GetBySlug(ctx, BusinessSlug)
	if errors.Is(err, tenant.ErrNotFound) {
		created, createErr := repo.Create(ctx, tenant.CreateBusinessParams{
			Name:     BusinessName,
			Slug:     BusinessSlug,
			Type:     tenant.BusinessTypeFNB,
			Timezone: BusinessTimezone,
		})
		if createErr != nil {
			return nil, fmt.Errorf("create business %q: %w", BusinessSlug, createErr)
		}
		return created, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup business %q: %w", BusinessSlug, err)
	}

	if business.Name != BusinessName ||
		business.Type != tenant.BusinessTypeFNB ||
		business.Timezone != BusinessTimezone ||
		business.Status != tenant.StatusActive {
		return nil, fmt.Errorf(
			"business %q already exists but does not match the development seed (expected name=%q type=%s timezone=%s status=%s)",
			BusinessSlug, BusinessName, tenant.BusinessTypeFNB, BusinessTimezone, tenant.StatusActive,
		)
	}

	return business, nil
}

func ensureUser(ctx context.Context, repo *tenant.UserRepository, in Input) (*tenant.User, error) {
	user, err := repo.GetByAuthUserID(ctx, in.AuthUserID)
	if errors.Is(err, tenant.ErrNotFound) {
		created, createErr := repo.Create(ctx, tenant.CreateUserParams{
			AuthUserID: in.AuthUserID,
			FullName:   UserFullName,
			Email:      in.Email,
		})
		if createErr != nil {
			if errors.Is(createErr, tenant.ErrConflict) {
				return nil, fmt.Errorf("a local user already uses email %q or auth_user_id %s", in.Email, in.AuthUserID)
			}
			return nil, fmt.Errorf("create user: %w", createErr)
		}
		return created, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	if user.Status != tenant.StatusActive {
		return nil, fmt.Errorf("local user %s already exists but is %s; the seeder will not reactivate it", user.ID, user.Status)
	}

	return user, nil
}

func ensureMembership(ctx context.Context, repo *tenant.BusinessUserRepository, businessID, userID, ownerRoleID uuid.UUID) (*tenant.BusinessUser, error) {
	membership, err := repo.Get(ctx, businessID, userID)
	if errors.Is(err, tenant.ErrNotFound) {
		created, createErr := repo.Create(ctx, tenant.CreateBusinessUserParams{
			BusinessID: businessID,
			UserID:     userID,
			RoleID:     ownerRoleID,
		})
		if createErr != nil {
			return nil, fmt.Errorf("create membership: %w", createErr)
		}
		return created, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup membership: %w", err)
	}

	if membership.RoleID != ownerRoleID {
		return nil, fmt.Errorf("membership %s already exists with a different role; the seeder will not change it", membership.ID)
	}
	if membership.Status != tenant.StatusActive {
		return nil, fmt.Errorf("membership %s already exists but is %s; the seeder will not reactivate it", membership.ID, membership.Status)
	}

	return membership, nil
}
