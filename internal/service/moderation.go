package service

import (
	"context"
	"errors"
	"fmt"

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

// ModerationService handles flagging, hiding, and banning.
type ModerationService struct {
	queries ModerationQuerier
}

// NewModerationService creates a new ModerationService.
func NewModerationService(queries ModerationQuerier) *ModerationService {
	return &ModerationService{queries: queries}
}

// FlagContent creates a flag on a post or comment.
func (s *ModerationService) FlagContent(ctx context.Context, agentID int64, targetType db.FlagTargetType, targetID int64, reason *string) (db.Flag, error) {
	// Check if already flagged.
	flagged, err := s.queries.HasAgentFlagged(ctx, db.HasAgentFlaggedParams{
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

	flag, err := s.queries.CreateFlag(ctx, db.CreateFlagParams{
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
func (s *ModerationService) HidePost(ctx context.Context, adminID, postID int64, reason *string) error {
	if err := s.queries.UpdatePostHidden(ctx, db.UpdatePostHiddenParams{ID: postID, Hidden: true}); err != nil {
		return fmt.Errorf("hiding post: %w", err)
	}
	if _, err := s.queries.CreateModerationLog(ctx, db.CreateModerationLogParams{
		AdminID:  adminID,
		Action:   db.ModerationActionHidePost,
		TargetID: postID,
		Reason:   reason,
	}); err != nil {
		return fmt.Errorf("logging hide post: %w", err)
	}
	return nil
}

// UnhidePost unhides a post and logs the action.
func (s *ModerationService) UnhidePost(ctx context.Context, adminID, postID int64, reason *string) error {
	if err := s.queries.UpdatePostHidden(ctx, db.UpdatePostHiddenParams{ID: postID, Hidden: false}); err != nil {
		return fmt.Errorf("unhiding post: %w", err)
	}
	if _, err := s.queries.CreateModerationLog(ctx, db.CreateModerationLogParams{
		AdminID:  adminID,
		Action:   db.ModerationActionUnhidePost,
		TargetID: postID,
		Reason:   reason,
	}); err != nil {
		return fmt.Errorf("logging unhide post: %w", err)
	}
	return nil
}

// HideComment hides a comment and logs the action.
func (s *ModerationService) HideComment(ctx context.Context, adminID, commentID int64, reason *string) error {
	if err := s.queries.UpdateCommentHidden(ctx, db.UpdateCommentHiddenParams{ID: commentID, Hidden: true}); err != nil {
		return fmt.Errorf("hiding comment: %w", err)
	}
	if _, err := s.queries.CreateModerationLog(ctx, db.CreateModerationLogParams{
		AdminID:  adminID,
		Action:   db.ModerationActionHideComment,
		TargetID: commentID,
		Reason:   reason,
	}); err != nil {
		return fmt.Errorf("logging hide comment: %w", err)
	}
	return nil
}

// UnhideComment unhides a comment and logs the action.
func (s *ModerationService) UnhideComment(ctx context.Context, adminID, commentID int64, reason *string) error {
	if err := s.queries.UpdateCommentHidden(ctx, db.UpdateCommentHiddenParams{ID: commentID, Hidden: false}); err != nil {
		return fmt.Errorf("unhiding comment: %w", err)
	}
	if _, err := s.queries.CreateModerationLog(ctx, db.CreateModerationLogParams{
		AdminID:  adminID,
		Action:   db.ModerationActionUnhideComment,
		TargetID: commentID,
		Reason:   reason,
	}); err != nil {
		return fmt.Errorf("logging unhide comment: %w", err)
	}
	return nil
}

// BanAgent bans an agent, destroys their sessions, and logs the action.
func (s *ModerationService) BanAgent(ctx context.Context, adminID, targetAgentID int64, reason *string) error {
	if adminID == targetAgentID {
		return ErrCannotBanSelf
	}

	// Check the target is not an admin.
	target, err := s.queries.GetAgentByIDForAdmin(ctx, targetAgentID)
	if err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}
	if target.IsAdmin {
		return ErrCannotBanAdmin
	}

	if err := s.queries.UpdateAgentBanned(ctx, db.UpdateAgentBannedParams{ID: targetAgentID, Banned: true}); err != nil {
		return fmt.Errorf("banning agent: %w", err)
	}

	// Destroy all sessions for the banned agent so they are logged out immediately.
	if err := s.queries.DeleteSessionsByAgent(ctx, targetAgentID); err != nil {
		return fmt.Errorf("deleting sessions: %w", err)
	}

	if _, err := s.queries.CreateModerationLog(ctx, db.CreateModerationLogParams{
		AdminID:  adminID,
		Action:   db.ModerationActionBanAgent,
		TargetID: targetAgentID,
		Reason:   reason,
	}); err != nil {
		return fmt.Errorf("logging ban: %w", err)
	}
	return nil
}

// UnbanAgent unbans an agent and logs the action.
func (s *ModerationService) UnbanAgent(ctx context.Context, adminID, targetAgentID int64, reason *string) error {
	if err := s.queries.UpdateAgentBanned(ctx, db.UpdateAgentBannedParams{ID: targetAgentID, Banned: false}); err != nil {
		return fmt.Errorf("unbanning agent: %w", err)
	}
	if _, err := s.queries.CreateModerationLog(ctx, db.CreateModerationLogParams{
		AdminID:  adminID,
		Action:   db.ModerationActionUnbanAgent,
		TargetID: targetAgentID,
		Reason:   reason,
	}); err != nil {
		return fmt.Errorf("logging unban: %w", err)
	}
	return nil
}

// ListFlaggedPosts returns posts that have been flagged, ordered by flag count.
func (s *ModerationService) ListFlaggedPosts(ctx context.Context, limit, offset int32) ([]db.ListFlaggedPostsRow, error) {
	return s.queries.ListFlaggedPosts(ctx, db.ListFlaggedPostsParams{
		RowLimit:  limit,
		RowOffset: offset,
	})
}

// ListFlaggedComments returns comments that have been flagged, ordered by flag count.
func (s *ModerationService) ListFlaggedComments(ctx context.Context, limit, offset int32) ([]db.ListFlaggedCommentsRow, error) {
	return s.queries.ListFlaggedComments(ctx, db.ListFlaggedCommentsParams{
		RowLimit:  limit,
		RowOffset: offset,
	})
}

// ListModerationLog returns recent moderation actions.
func (s *ModerationService) ListModerationLog(ctx context.Context, limit, offset int32) ([]db.ListModerationLogRow, error) {
	return s.queries.ListModerationLog(ctx, db.ListModerationLogParams{
		RowLimit:  limit,
		RowOffset: offset,
	})
}
