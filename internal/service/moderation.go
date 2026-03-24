package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	db "github.com/trishtzy/warren/db/generated"
)

var (
	ErrNotAdmin       = errors.New("admin access required")
	ErrCannotBanAdmin = errors.New("cannot ban an admin")
	ErrCannotBanSelf  = errors.New("cannot ban yourself")
	ErrAlreadyFlagged = errors.New("you have already flagged this content")
	ErrAccountBanned  = errors.New("your account has been suspended")
)

// ModerationQuerier defines the database methods required by ModerationService.
type ModerationQuerier interface {
	CreateFlag(ctx context.Context, arg db.CreateFlagParams) (db.Flag, error)
	HasAgentFlagged(ctx context.Context, arg db.HasAgentFlaggedParams) (bool, error)
	ListFlaggedPosts(ctx context.Context, arg db.ListFlaggedPostsParams) ([]db.ListFlaggedPostsRow, error)
	ListFlaggedComments(ctx context.Context, arg db.ListFlaggedCommentsParams) ([]db.ListFlaggedCommentsRow, error)
	UpdatePostHidden(ctx context.Context, arg db.UpdatePostHiddenParams) error
	UpdateCommentHidden(ctx context.Context, arg db.UpdateCommentHiddenParams) error
	UpdateAgentBanned(ctx context.Context, arg db.UpdateAgentBannedParams) error
	DeleteSessionsByAgent(ctx context.Context, agentID int64) error
	GetAgentByIDForAdmin(ctx context.Context, id int64) (db.GetAgentByIDForAdminRow, error)
	CreateModerationLog(ctx context.Context, arg db.CreateModerationLogParams) (db.ModerationLog, error)
	ListModerationLog(ctx context.Context, arg db.ListModerationLogParams) ([]db.ListModerationLogRow, error)
}

// ModerationStore extends ModerationQuerier with transaction support.
type ModerationStore interface {
	ModerationQuerier
	// ExecTx runs fn within a database transaction, passing a transactional ModerationQuerier.
	// The transaction is committed if fn returns nil, rolled back otherwise.
	ExecTx(ctx context.Context, fn func(ModerationQuerier) error) error
}

// PgModerationStore wraps a db.Queries and pgx pool to implement ModerationStore.
type PgModerationStore struct {
	*db.Queries
	pool interface {
		Begin(ctx context.Context) (pgx.Tx, error)
	}
}

// NewPgModerationStore creates a PgModerationStore from a pool.
func NewPgModerationStore(queries *db.Queries, pool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}) *PgModerationStore {
	return &PgModerationStore{Queries: queries, pool: pool}
}

// ExecTx begins a transaction, calls fn with a transactional Queries, and commits or rolls back.
func (s *PgModerationStore) ExecTx(ctx context.Context, fn func(ModerationQuerier) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.WithTx(tx)
	if err := fn(qtx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ModerationService handles flagging, hiding, and banning.
type ModerationService struct {
	store ModerationStore
}

// NewModerationService creates a new ModerationService.
func NewModerationService(store ModerationStore) *ModerationService {
	return &ModerationService{store: store}
}

// FlagContent creates a flag on a post or comment.
func (s *ModerationService) FlagContent(ctx context.Context, agentID int64, targetType db.FlagTargetType, targetID int64, reason *string) (db.Flag, error) {
	// Check if already flagged.
	flagged, err := s.store.HasAgentFlagged(ctx, db.HasAgentFlaggedParams{
		AgentID:    agentID,
		TargetType: targetType,
		TargetID:   targetID,
	})
	if err != nil {
		return db.Flag{}, fmt.Errorf("checking flag: %w", err)
	}
	if flagged {
		return db.Flag{}, ErrAlreadyFlagged
	}

	flag, err := s.store.CreateFlag(ctx, db.CreateFlagParams{
		AgentID:    agentID,
		TargetType: targetType,
		TargetID:   targetID,
		Reason:     reason,
	})
	if err != nil {
		return db.Flag{}, fmt.Errorf("creating flag: %w", err)
	}
	return flag, nil
}

// HidePost hides a post and logs the action.
// Both writes run inside a single transaction to ensure atomicity.
func (s *ModerationService) HidePost(ctx context.Context, adminID, postID int64, reason *string) error {
	return s.store.ExecTx(ctx, func(q ModerationQuerier) error {
		if err := q.UpdatePostHidden(ctx, db.UpdatePostHiddenParams{ID: postID, Hidden: true}); err != nil {
			return fmt.Errorf("hiding post: %w", err)
		}
		if _, err := q.CreateModerationLog(ctx, db.CreateModerationLogParams{
			AdminID:  adminID,
			Action:   db.ModerationActionHidePost,
			TargetID: postID,
			Reason:   reason,
		}); err != nil {
			return fmt.Errorf("logging hide post: %w", err)
		}
		return nil
	})
}

// UnhidePost unhides a post and logs the action.
// Both writes run inside a single transaction to ensure atomicity.
func (s *ModerationService) UnhidePost(ctx context.Context, adminID, postID int64, reason *string) error {
	return s.store.ExecTx(ctx, func(q ModerationQuerier) error {
		if err := q.UpdatePostHidden(ctx, db.UpdatePostHiddenParams{ID: postID, Hidden: false}); err != nil {
			return fmt.Errorf("unhiding post: %w", err)
		}
		if _, err := q.CreateModerationLog(ctx, db.CreateModerationLogParams{
			AdminID:  adminID,
			Action:   db.ModerationActionUnhidePost,
			TargetID: postID,
			Reason:   reason,
		}); err != nil {
			return fmt.Errorf("logging unhide post: %w", err)
		}
		return nil
	})
}

// HideComment hides a comment and logs the action.
// Both writes run inside a single transaction to ensure atomicity.
func (s *ModerationService) HideComment(ctx context.Context, adminID, commentID int64, reason *string) error {
	return s.store.ExecTx(ctx, func(q ModerationQuerier) error {
		if err := q.UpdateCommentHidden(ctx, db.UpdateCommentHiddenParams{ID: commentID, Hidden: true}); err != nil {
			return fmt.Errorf("hiding comment: %w", err)
		}
		if _, err := q.CreateModerationLog(ctx, db.CreateModerationLogParams{
			AdminID:  adminID,
			Action:   db.ModerationActionHideComment,
			TargetID: commentID,
			Reason:   reason,
		}); err != nil {
			return fmt.Errorf("logging hide comment: %w", err)
		}
		return nil
	})
}

// UnhideComment unhides a comment and logs the action.
// Both writes run inside a single transaction to ensure atomicity.
func (s *ModerationService) UnhideComment(ctx context.Context, adminID, commentID int64, reason *string) error {
	return s.store.ExecTx(ctx, func(q ModerationQuerier) error {
		if err := q.UpdateCommentHidden(ctx, db.UpdateCommentHiddenParams{ID: commentID, Hidden: false}); err != nil {
			return fmt.Errorf("unhiding comment: %w", err)
		}
		if _, err := q.CreateModerationLog(ctx, db.CreateModerationLogParams{
			AdminID:  adminID,
			Action:   db.ModerationActionUnhideComment,
			TargetID: commentID,
			Reason:   reason,
		}); err != nil {
			return fmt.Errorf("logging unhide comment: %w", err)
		}
		return nil
	})
}

// BanAgent bans an agent, destroys their sessions, and logs the action.
// All three writes run inside a single transaction to ensure atomicity.
func (s *ModerationService) BanAgent(ctx context.Context, adminID, targetAgentID int64, reason *string) error {
	if adminID == targetAgentID {
		return ErrCannotBanSelf
	}

	// Check the target is not an admin (read outside the tx is fine — it's a guard check).
	target, err := s.store.GetAgentByIDForAdmin(ctx, targetAgentID)
	if err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}
	if target.IsAdmin {
		return ErrCannotBanAdmin
	}

	return s.store.ExecTx(ctx, func(q ModerationQuerier) error {
		if err := q.UpdateAgentBanned(ctx, db.UpdateAgentBannedParams{ID: targetAgentID, Banned: true}); err != nil {
			return fmt.Errorf("banning agent: %w", err)
		}

		// Destroy all sessions for the banned agent so they are logged out immediately.
		if err := q.DeleteSessionsByAgent(ctx, targetAgentID); err != nil {
			return fmt.Errorf("deleting sessions: %w", err)
		}

		if _, err := q.CreateModerationLog(ctx, db.CreateModerationLogParams{
			AdminID:  adminID,
			Action:   db.ModerationActionBanAgent,
			TargetID: targetAgentID,
			Reason:   reason,
		}); err != nil {
			return fmt.Errorf("logging ban: %w", err)
		}
		return nil
	})
}

// UnbanAgent unbans an agent and logs the action.
// Both writes run inside a single transaction to ensure atomicity.
func (s *ModerationService) UnbanAgent(ctx context.Context, adminID, targetAgentID int64, reason *string) error {
	return s.store.ExecTx(ctx, func(q ModerationQuerier) error {
		if err := q.UpdateAgentBanned(ctx, db.UpdateAgentBannedParams{ID: targetAgentID, Banned: false}); err != nil {
			return fmt.Errorf("unbanning agent: %w", err)
		}
		if _, err := q.CreateModerationLog(ctx, db.CreateModerationLogParams{
			AdminID:  adminID,
			Action:   db.ModerationActionUnbanAgent,
			TargetID: targetAgentID,
			Reason:   reason,
		}); err != nil {
			return fmt.Errorf("logging unban: %w", err)
		}
		return nil
	})
}

// ListFlaggedPosts returns posts that have been flagged, ordered by flag count.
func (s *ModerationService) ListFlaggedPosts(ctx context.Context, limit, offset int32) ([]db.ListFlaggedPostsRow, error) {
	return s.store.ListFlaggedPosts(ctx, db.ListFlaggedPostsParams{
		RowLimit:  limit,
		RowOffset: offset,
	})
}

// ListFlaggedComments returns comments that have been flagged, ordered by flag count.
func (s *ModerationService) ListFlaggedComments(ctx context.Context, limit, offset int32) ([]db.ListFlaggedCommentsRow, error) {
	return s.store.ListFlaggedComments(ctx, db.ListFlaggedCommentsParams{
		RowLimit:  limit,
		RowOffset: offset,
	})
}

// ListModerationLog returns recent moderation actions.
func (s *ModerationService) ListModerationLog(ctx context.Context, limit, offset int32) ([]db.ListModerationLogRow, error) {
	return s.store.ListModerationLog(ctx, db.ListModerationLogParams{
		RowLimit:  limit,
		RowOffset: offset,
	})
}
