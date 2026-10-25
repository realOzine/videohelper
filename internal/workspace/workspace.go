package workspace

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/platform/authn"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/workspace/workspacedb"
)

type Role string

const (
	Owner  Role = "owner"
	Member Role = "member"
)

type Principal struct {
	UserID, WorkspaceID int64
	Role                Role
}

type Service struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Resolve accepts only claims from a verified token. Provisioning and membership selection are atomic.
func (s *Service) Resolve(ctx context.Context, claims authn.Claims, requested int64) (Principal, error) {
	if strings.TrimSpace(claims.Sub) == "" {
		return Principal{}, apperr.New(apperr.Unauthenticated, "invalid_identity", "认证身份缺少 sub")
	}
	if requested < 0 {
		return Principal{}, apperr.New(apperr.Invalid, "invalid_workspace", "工作区 ID 必须为正整数")
	}
	var principal Principal
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := workspacedb.New(tx)
		userID, err := resolveUser(ctx, q, claims)
		if err != nil {
			return err
		}
		principal.UserID = userID
		if requested > 0 {
			role, err := q.MembershipForWorkspace(ctx, workspacedb.MembershipForWorkspaceParams{UserID: userID, WorkspaceID: requested})
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(apperr.Forbidden, "workspace_forbidden", "没有访问该工作区的权限")
			}
			principal.WorkspaceID, principal.Role = requested, Role(role)
			return err
		}
		memberships, err := q.MembershipsForUser(ctx, userID)
		if err != nil {
			return err
		}
		switch len(memberships) {
		case 0:
			return apperr.New(apperr.Forbidden, "workspace_required", "没有可访问的工作区")
		case 1:
			principal.WorkspaceID, principal.Role = memberships[0].WorkspaceID, Role(memberships[0].Role)
			return nil
		default:
			return apperr.New(apperr.Invalid, "workspace_required", "请通过 X-Workspace-ID 选择工作区")
		}
	})
	if err != nil {
		return Principal{}, err
	}
	return principal, nil
}

func resolveUser(ctx context.Context, q *workspacedb.Queries, claims authn.Claims) (int64, error) {
	id, err := q.CreateUser(ctx, workspacedb.CreateUserParams{AuthSub: claims.Sub, Email: claims.Email, Name: claims.Name})
	if errors.Is(err, pgx.ErrNoRows) {
		return q.UserBySubject(ctx, claims.Sub)
	}
	if err != nil {
		return 0, err
	}
	name := "个人工作区"
	if strings.TrimSpace(claims.Name) != "" {
		name = claims.Name + "的工作区"
	}
	workspaceID, err := q.CreateWorkspace(ctx, name)
	if err != nil {
		return 0, err
	}
	err = q.AddOwner(ctx, workspacedb.AddOwnerParams{WorkspaceID: workspaceID, UserID: id})
	return id, err
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok
}
