package workitems

import (
	"context"
	"strings"
	"time"
)

// TeamAuthority is the one thing this package needs from the teams package
// to scope a supervisor's actions — defined here, at the point of
// consumption, and satisfied structurally by teams.Service with no import
// of teams needed on either side ("accept interfaces, return structs").
type TeamAuthority interface {
	IsActiveSupervisorOf(ctx context.Context, supervisorUserID, assigneeUserID string) (bool, error)
	SupervisedAssigneeUserIDs(ctx context.Context, supervisorUserID string) ([]string, error)
}

// NotificationSink is the seam OPS-041 uses to raise a notification as a
// side effect of a workflow event, without workitems depending on the
// notifications package concretely. Defined here, where it's consumed,
// rather than in the notifications package, where it's implemented —
// notifications.Service satisfies this by having a matching Notify
// method, with no import or explicit "implements" declaration needed on
// either side. kind is a plain string on purpose: workitems never needs
// to know notifications.Kind exists, only that it's calling Notify with
// one of a few string literals.
type NotificationSink interface {
	Notify(ctx context.Context, recipientUserID, workItemID, kind, message string) error
}

// TxRunner lets a multi-store workflow method (AssignWorkItem, ChangeStatus,
// RespondToAssignment — every method that writes to more than one store in
// a single call) run its whole sequence of store writes as one atomic
// unit, when the underlying stores support real transactions. Defined
// here, at the point of consumption, satisfied structurally by
// db.PoolTxRunner with no import of db needed here. nil (the default —
// what every Service built with the in-memory stores gets, since there's
// nothing there for a transaction to make atomic: independent mutex-guarded
// map writes don't have the partial-failure mode multi-statement SQL does)
// is treated as "no transaction support," and every method that uses it
// degrades to calling straight through — exactly the behavior every
// caller had before this existed.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Service struct {
	store                  Store
	historyStore           StatusHistoryStore
	assignmentStore        AssignmentStore
	assignmentHistoryStore AssignmentHistoryStore
	notifier               NotificationSink
	teamAuthority          TeamAuthority
	txRunner               TxRunner
	now                    func() time.Time
}

func NewService(store Store, historyStore StatusHistoryStore, assignmentStore AssignmentStore, assignmentHistoryStore AssignmentHistoryStore, notifier NotificationSink, teamAuthority TeamAuthority, txRunner TxRunner) Service {
	return Service{
		store:                  store,
		historyStore:           historyStore,
		assignmentStore:        assignmentStore,
		assignmentHistoryStore: assignmentHistoryStore,
		notifier:               notifier,
		teamAuthority:          teamAuthority,
		txRunner:               txRunner,
		now:                    time.Now,
	}
}

// runAtomic is the one place ChangeStatus/AssignWorkItem/RespondToAssignment
// reach for transactional grouping — see TxRunner's doc comment for why nil
// is a safe, common, and correct value here.
func (s Service) runAtomic(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txRunner == nil {
		return fn(ctx)
	}

	return s.txRunner.WithTx(ctx, fn)
}

func (s Service) Create(ctx context.Context, createdByUserID string, input CreateInput) (WorkItem, error) {
	if strings.TrimSpace(createdByUserID) == "" {
		return WorkItem{}, ErrInvalidInput
	}

	title := strings.TrimSpace(input.Title)
	description := strings.TrimSpace(input.Description)

	if title == "" || description == "" {
		return WorkItem{}, ErrInvalidInput
	}

	if !input.Priority.IsValid() {
		return WorkItem{}, ErrInvalidPriority
	}

	now := s.now()

	item := WorkItem{
		Title:           title,
		Description:     description,
		Status:          StatusCreated,
		Priority:        input.Priority,
		CreatedByUserID: createdByUserID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if input.LocationText != nil {
		location := strings.TrimSpace(*input.LocationText)
		if location != "" {
			item.LocationText = &location
		}
	}

	if input.DueAt != nil {
		item.DueAt = ptrTime(*input.DueAt)
	}

	return s.store.Create(ctx, item)
}

// List returns work items scoped to the caller. An admin sees every work
// item. A supervisor (OPS-045) sees work items currently assigned to
// anyone on a team they actively supervise, plus their own not-yet-assigned
// work — the same "own team, plus own unassigned drafts" shape
// supervisorMayActOn uses for actions. A requester (OPS-047) sees only
// work items they created — they never have anything assigned to them,
// so ListByCreatedByUserID rather than ListByAssignedToUserID is the
// right scope. A plain caller (assignee) only sees work items currently
// assigned to them — the "assignee sees own assigned work only" rule from
// the starter's permission matrix. The workitems package deliberately
// doesn't import the auth package, so the caller (the HTTP handler, which
// does know about roles) resolves "is this an admin / supervisor /
// requester" into plain bools before calling this method.
func (s Service) List(ctx context.Context, callerUserID string, callerIsAdmin bool, callerIsSupervisor bool, callerIsRequester bool) ([]WorkItem, error) {
	if callerIsAdmin {
		return s.store.List(ctx)
	}

	if strings.TrimSpace(callerUserID) == "" {
		return nil, ErrInvalidInput
	}

	if callerIsSupervisor {
		return s.listForSupervisor(ctx, callerUserID)
	}

	if callerIsRequester {
		return s.store.ListByCreatedByUserID(ctx, callerUserID)
	}

	return s.store.ListByAssignedToUserID(ctx, callerUserID)
}

// listForSupervisor filters every work item down to the ones a supervisor
// may see: assigned to someone on a team they supervise, or created by them
// and not yet assigned to anyone. It walks the full store rather than
// adding a new indexed lookup — the starter's in-memory stores are small by
// design, and a real persistence layer would express this as a WHERE
// clause instead of an in-process filter.
func (s Service) listForSupervisor(ctx context.Context, callerUserID string) ([]WorkItem, error) {
	all, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}

	var supervisedAssigneeIDs []string
	if s.teamAuthority != nil {
		supervisedAssigneeIDs, err = s.teamAuthority.SupervisedAssigneeUserIDs(ctx, callerUserID)
		if err != nil {
			return nil, err
		}
	}

	supervised := make(map[string]bool, len(supervisedAssigneeIDs))
	for _, userID := range supervisedAssigneeIDs {
		supervised[userID] = true
	}

	visible := make([]WorkItem, 0)
	for _, item := range all {
		if item.AssignedToUserID != nil {
			if supervised[*item.AssignedToUserID] {
				visible = append(visible, item)
			}
			continue
		}

		if item.CreatedByUserID == callerUserID {
			visible = append(visible, item)
		}
	}

	return visible, nil
}

// GetByID returns one work item, scoped the same way as List. A caller who
// cannot see a work item gets ErrNotFound, not a "forbidden" error — from
// their point of view the work item does not exist, rather than
// existing-but-hidden. This avoids revealing that a given id belongs to
// someone else.
func (s Service) GetByID(ctx context.Context, id string, callerUserID string, callerIsAdmin bool, callerIsSupervisor bool, callerIsRequester bool) (WorkItem, error) {
	if strings.TrimSpace(id) == "" {
		return WorkItem{}, ErrInvalidInput
	}

	item, err := s.store.GetByID(ctx, id)
	if err != nil {
		return WorkItem{}, err
	}

	if callerIsAdmin {
		return item, nil
	}

	if callerIsRequester {
		if item.CreatedByUserID != callerUserID {
			return WorkItem{}, ErrNotFound
		}

		return item, nil
	}

	if callerIsSupervisor {
		allowed, err := s.supervisorMayActOn(ctx, callerUserID, item)
		if err != nil {
			return WorkItem{}, err
		}
		if !allowed {
			return WorkItem{}, ErrNotFound
		}

		return item, nil
	}

	if item.AssignedToUserID == nil || *item.AssignedToUserID != callerUserID {
		return WorkItem{}, ErrNotFound
	}

	return item, nil
}

// Update edits a work item's editable fields, scoped for OPS-046: admin can
// edit anything at any status, a supervisor can only edit their own team's
// work items — and, per the permission matrix's literal action name
// ("Edit unassigned work item"), only while the item is still unassigned.
// Once a supervisor hands a work item off, its title/description/priority
// stop being theirs to rewrite; admin keeps the broader, unrestricted-at-
// any-status latitude it already has everywhere else in this package. A
// review on PR #54 caught this: supervisorMayActOn alone (reused
// everywhere else for team-scoped actions) checks *who* may act, not
// *when* — its assigned-item branch has no status check, so without this
// extra gate a supervisor could keep editing a work item's core fields
// straight through verified/completed. This deliberately does not reuse
// mayView either: mayView grants an assignee read access to their own
// assigned item, correct for viewing but wrong for editing — Update has
// no assignee path at all. Before OPS-046 this had no caller/ownership
// check whatsoever — any admin-gated caller could edit any work item at
// any status — which happened to be harmless while the route was
// admin-only, but became a real gap the moment the route needed to open
// up to supervisor as well.
func (s Service) Update(ctx context.Context, id string, callerUserID string, callerIsAdmin bool, callerIsSupervisor bool, input UpdateInput) (WorkItem, error) {
	if strings.TrimSpace(id) == "" {
		return WorkItem{}, ErrInvalidInput
	}

	item, err := s.store.GetByID(ctx, id)
	if err != nil {
		return WorkItem{}, err
	}

	switch {
	case callerIsAdmin:
		// unrestricted, any status
	case callerIsSupervisor:
		// Authority (can this supervisor see/act on this item at all)
		// is checked before the assigned-status business rule below —
		// same ordering ChangeStatus already uses (ownership before
		// transition legality) — so a supervisor with no authority over
		// this item at all still gets ErrNotFound, not a hint that the
		// item exists and is merely "already assigned."
		allowed, err := s.supervisorMayActOn(ctx, callerUserID, item)
		if err != nil {
			return WorkItem{}, err
		}
		if !allowed {
			return WorkItem{}, ErrNotFound
		}

		if item.AssignedToUserID != nil {
			return WorkItem{}, ErrAlreadyAssigned
		}
	default:
		return WorkItem{}, ErrNotFound
	}

	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" {
			return WorkItem{}, ErrInvalidInput
		}

		item.Title = title
	}

	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		if description == "" {
			return WorkItem{}, ErrInvalidInput
		}

		item.Description = description
	}

	if input.Priority != nil {
		if !input.Priority.IsValid() {
			return WorkItem{}, ErrInvalidPriority
		}

		item.Priority = *input.Priority
	}

	if input.LocationText != nil {
		location := strings.TrimSpace(*input.LocationText)
		if location == "" {
			item.LocationText = nil
		} else {
			item.LocationText = &location
		}
	}

	if input.DueAt != nil {
		item.DueAt = ptrTime(*input.DueAt)
	}

	item.UpdatedAt = s.now()

	return s.store.Update(ctx, item)
}

// ChangeStatus moves a work item to a new status, checks the move is legal,
// saves the new status on the work item, and writes a StatusHistory entry
// recording what changed, who changed it, and why.
//
// An admin caller may trigger any transition IsValidTransition allows, on
// any work item, unrestricted.
//
// A supervisor caller (OPS-045) gets the same transition set as admin — not
// the narrower assignee list — but only on work items currently assigned to
// someone on a team they actively supervise, checked via teamAuthority.
// An unassigned item has no team to check against, so a supervisor may only
// act on it if they created it themselves.
//
// A plain assignee caller is restricted twice: the work item must actually
// be assigned to them (ErrNotFound otherwise, same "acts as if it doesn't
// exist" rule as GetByID), and the move itself must be one of the few
// IsAssigneeAllowedTransition grants — issue #42, "start work" and "submit
// progress update" from the permission matrix, nothing wider.
func (s Service) ChangeStatus(ctx context.Context, id string, actorUserID string, actorIsAdmin bool, actorIsSupervisor bool, input ChangeStatusInput) (WorkItem, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(actorUserID) == "" {
		return WorkItem{}, ErrInvalidInput
	}

	if !input.ToStatus.IsValid() {
		return WorkItem{}, ErrInvalidStatus
	}

	// Flagging without saying why leaves the assignee with nothing to act
	// on (see FlagInput's doc comment). The dedicated POST .../flag
	// handler already enforces this before it ever calls ChangeStatus,
	// but that only covers callers going through that one route — the
	// generic PATCH .../status endpoint reaches this same transition with
	// no such check. Enforcing it here closes it for every entry point
	// and every caller (admin included), not just the one handler.
	if input.ToStatus == StatusFlagged && (input.Reason == nil || strings.TrimSpace(*input.Reason) == "") {
		return WorkItem{}, ErrFeedbackRequired
	}

	// Everything from here on writes to at least two stores (work_items,
	// status_history, and sometimes notifications) — wrapped in runAtomic
	// so a Postgres-backed Service either commits all of it or none of it.
	// See TxRunner's doc comment for why nil (in-memory stores) just runs
	// this closure directly with no transaction involved.
	var updated WorkItem

	err := s.runAtomic(ctx, func(ctx context.Context) error {
		item, err := s.store.GetByID(ctx, id)
		if err != nil {
			return err
		}

		switch {
		case actorIsAdmin:
			if !IsValidTransition(item.Status, input.ToStatus) {
				return ErrInvalidTransition
			}
		case actorIsSupervisor:
			allowed, err := s.supervisorMayActOn(ctx, actorUserID, item)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrNotFound
			}

			if !IsValidTransition(item.Status, input.ToStatus) {
				return ErrInvalidTransition
			}
		default:
			if item.AssignedToUserID == nil || *item.AssignedToUserID != actorUserID {
				return ErrNotFound
			}

			if !IsAssigneeAllowedTransition(item.Status, input.ToStatus) {
				return ErrInvalidTransition
			}
		}

		fromStatus := item.Status
		now := s.now()

		item.Status = input.ToStatus
		item.UpdatedAt = now

		updated, err = s.store.Update(ctx, item)
		if err != nil {
			return err
		}

		if err := s.recordStatusChange(ctx, id, fromStatus, input.ToStatus, actorUserID, input.Reason, now); err != nil {
			return err
		}

		if recipientUserID, kind, message, ok := notificationForStatusChange(updated); ok {
			if err := s.notify(ctx, recipientUserID, id, kind, message); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return WorkItem{}, err
	}

	return updated, nil
}

// supervisorMayActOn reports whether a supervisor caller has authority over
// a specific work item: either it's currently assigned to someone on a team
// they actively supervise, or it isn't assigned to anyone yet and they're
// the one who created it (their own not-yet-assigned work). teamAuthority
// being nil is treated as "no team authority configured" rather than a
// panic, so a Service built without it (e.g. an older test) degrades to
// "supervisors can only act on their own unassigned work" instead of
// crashing.
func (s Service) supervisorMayActOn(ctx context.Context, actorUserID string, item WorkItem) (bool, error) {
	if item.AssignedToUserID == nil {
		return item.CreatedByUserID == actorUserID, nil
	}

	if s.teamAuthority == nil {
		return false, nil
	}

	return s.teamAuthority.IsActiveSupervisorOf(ctx, actorUserID, *item.AssignedToUserID)
}

// notificationForStatusChange maps the four Event-Hooks-listed status
// destinations to who should hear about it. Every other destination
// (accepted, in_progress, created, cancelled) returns ok=false — starting
// work isn't a checkpoint anyone is waiting on the way these four are.
// Takes the already-updated WorkItem rather than a bare Status so the
// recipient (creator or assignee, depending on which event) is read off
// real data instead of being threaded through as extra parameters.
func notificationForStatusChange(item WorkItem) (recipientUserID, kind, message string, ok bool) {
	switch item.Status {
	case StatusSubmittedForReview:
		return item.CreatedByUserID, "evidence_submitted", item.ReferenceCode + " was submitted for review", true
	case StatusFlagged:
		if item.AssignedToUserID == nil {
			return "", "", "", false
		}
		return *item.AssignedToUserID, "work_flagged", item.ReferenceCode + " was flagged and needs rework", true
	case StatusVerified:
		if item.AssignedToUserID == nil {
			return "", "", "", false
		}
		return *item.AssignedToUserID, "work_verified", item.ReferenceCode + " was verified", true
	case StatusCompleted:
		if item.AssignedToUserID == nil {
			return "", "", "", false
		}
		return *item.AssignedToUserID, "work_completed", item.ReferenceCode + " was marked completed", true
	default:
		return "", "", "", false
	}
}

// recordStatusChange writes one StatusHistory entry. It is the single place
// that turns a status move into a history row, so ChangeStatus and
// AssignWorkItem (which also moves a status, from Created to Assigned)
// don't each keep their own copy of this logic.
func (s Service) recordStatusChange(ctx context.Context, workItemID string, from, to Status, actorUserID string, rawReason *string, at time.Time) error {
	var reason *string
	if rawReason != nil {
		trimmed := strings.TrimSpace(*rawReason)
		if trimmed != "" {
			reason = &trimmed
		}
	}

	_, err := s.historyStore.Create(ctx, StatusHistory{
		WorkItemID:      workItemID,
		FromStatus:      &from,
		ToStatus:        to,
		ChangedByUserID: actorUserID,
		Reason:          reason,
		CreatedAt:       at,
	})

	return err
}

// recordAssignmentEvent writes one AssignmentHistory entry — the
// AssignmentHistory equivalent of recordStatusChange above. Both
// AssignWorkItem and RespondToAssignment call this instead of each
// keeping their own copy of "how to turn an assignment event into a
// history row."
func (s Service) recordAssignmentEvent(ctx context.Context, workItemID string, action AssignmentStatus, actorUserID string, assignedToUserID string, rawNote *string, at time.Time) error {
	var note *string
	if rawNote != nil {
		trimmed := strings.TrimSpace(*rawNote)
		if trimmed != "" {
			note = &trimmed
		}
	}

	_, err := s.assignmentHistoryStore.Create(ctx, AssignmentHistory{
		WorkItemID:       workItemID,
		Action:           action,
		ActorUserID:      actorUserID,
		AssignedToUserID: assignedToUserID,
		Note:             note,
		CreatedAt:        at,
	})

	return err
}

// mayView is the shared "can this caller see this work item" check reused
// by ListStatusHistory, ListAssignmentHistory, and GetAssignment — the same
// three-way rule GetByID applies: admin sees anything, a supervisor sees
// their own team's items (via supervisorMayActOn), everyone else only sees
// work assigned to them.
func (s Service) mayView(ctx context.Context, item WorkItem, callerUserID string, callerIsAdmin bool, callerIsSupervisor bool) (bool, error) {
	if callerIsAdmin {
		return true, nil
	}

	if callerIsSupervisor {
		return s.supervisorMayActOn(ctx, callerUserID, item)
	}

	return item.AssignedToUserID != nil && *item.AssignedToUserID == callerUserID, nil
}

// notify is a small nil-safe wrapper around s.notifier.Notify — every
// call site already has a workItemID and a human-readable message ready,
// this just guards against notifier being unset (a caller that
// deliberately omits notifications, e.g. a test that doesn't care about
// them) so nothing panics on a nil interface.
func (s Service) notify(ctx context.Context, recipientUserID, workItemID, kind, message string) error {
	if s.notifier == nil {
		return nil
	}

	return s.notifier.Notify(ctx, recipientUserID, workItemID, kind, message)
}

// ListStatusHistory returns every status change recorded for a work item,
// oldest first. It confirms the work item exists before checking history so
// callers get a consistent ErrNotFound instead of an empty list for a
// missing id. A caller who cannot view the work item (see mayView) gets the
// same ErrNotFound-not-forbidden shape as GetByID.
func (s Service) ListStatusHistory(ctx context.Context, workItemID string, callerUserID string, callerIsAdmin bool, callerIsSupervisor bool) ([]StatusHistory, error) {
	if strings.TrimSpace(workItemID) == "" {
		return nil, ErrInvalidInput
	}

	item, err := s.store.GetByID(ctx, workItemID)
	if err != nil {
		return nil, err
	}

	allowed, err := s.mayView(ctx, item, callerUserID, callerIsAdmin, callerIsSupervisor)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrNotFound
	}

	return s.historyStore.ListByWorkItemID(ctx, workItemID)
}

// ListAssignmentHistory returns every assignment event recorded for a
// work item, oldest first — the OPS-040 audit trail. Scoped identically to
// ListStatusHistory via mayView.
func (s Service) ListAssignmentHistory(ctx context.Context, workItemID string, callerUserID string, callerIsAdmin bool, callerIsSupervisor bool) ([]AssignmentHistory, error) {
	if strings.TrimSpace(workItemID) == "" {
		return nil, ErrInvalidInput
	}

	item, err := s.store.GetByID(ctx, workItemID)
	if err != nil {
		return nil, err
	}

	allowed, err := s.mayView(ctx, item, callerUserID, callerIsAdmin, callerIsSupervisor)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrNotFound
	}

	return s.assignmentHistoryStore.ListByWorkItemID(ctx, workItemID)
}

// AssignWorkItem gives a work item to an assignee. It reuses the same
// status-transition rules as ChangeStatus: a work item can only move into
// StatusAssigned from a status that already allows it (StatusCreated), so
// a work item that has already been assigned, or moved further along, is
// rejected by the same IsValidTransition check rather than a new rule.
//
// A supervisor caller (OPS-045) is checked two ways, both of which must
// pass: supervisorMayActOn confirms they may act on this specific work
// item at all (it's their own not-yet-assigned creation — an unassigned
// item has no team to check against yet), and teamAuthority confirms the
// destination assigneeToUserID is on a team they actively supervise. The
// first check alone would let a supervisor "adopt" someone else's hidden
// unassigned item just by knowing its id and pointing it at their own
// team — supervisorMayActOn closes that: only the item's own creator (or
// admin) may assign it in the first place. An admin caller is
// unrestricted, as before.
//
// assignedByIsAdmin exists explicitly (OPS-047), rather than treating
// "not a supervisor" as "must be admin," the way this method's signature
// used to work. That shortcut was safe only as long as exactly two roles
// could ever reach this method — the moment a third, unprivileged role
// (requester) existed, "not supervisor" silently meant "unrestricted,"
// which would have let a requester assign anyone to anything. The default
// case below now explicitly rejects any caller who is neither.
func (s Service) AssignWorkItem(ctx context.Context, workItemID string, assignedByUserID string, assignedByIsAdmin bool, assignedByIsSupervisor bool, input AssignInput) (Assignment, error) {
	assignedToUserID := strings.TrimSpace(input.AssignedToUserID)

	if strings.TrimSpace(workItemID) == "" || strings.TrimSpace(assignedByUserID) == "" || assignedToUserID == "" {
		return Assignment{}, ErrInvalidInput
	}

	// Same reasoning as ChangeStatus: this writes to work_items,
	// status_history, assignments, assignment_history, and notifications
	// — four different stores — so it runs inside runAtomic. This is the
	// exact scenario a review on PR #57 called out by name: without this,
	// a failure on (say) the assignment_history write left the work item
	// already marked assigned with no assignment record at all, and a
	// retry got rejected by IsValidTransition below since the item wasn't
	// in StatusCreated anymore.
	var assignment Assignment

	err := s.runAtomic(ctx, func(ctx context.Context) error {
		item, err := s.store.GetByID(ctx, workItemID)
		if err != nil {
			return err
		}

		switch {
		case assignedByIsAdmin:
			// unrestricted
		case assignedByIsSupervisor:
			allowed, err := s.supervisorMayActOn(ctx, assignedByUserID, item)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrNotFound
			}

			if s.teamAuthority == nil {
				return ErrNotFound
			}

			destinationAllowed, err := s.teamAuthority.IsActiveSupervisorOf(ctx, assignedByUserID, assignedToUserID)
			if err != nil {
				return err
			}
			if !destinationAllowed {
				return ErrNotFound
			}
		default:
			return ErrNotFound
		}

		if !IsValidTransition(item.Status, StatusAssigned) {
			return ErrInvalidTransition
		}

		fromStatus := item.Status
		now := s.now()

		item.Status = StatusAssigned
		item.AssignedToUserID = &assignedToUserID
		item.UpdatedAt = now

		if _, err := s.store.Update(ctx, item); err != nil {
			return err
		}

		if err := s.recordStatusChange(ctx, workItemID, fromStatus, StatusAssigned, assignedByUserID, nil, now); err != nil {
			return err
		}

		assignment, err = s.assignmentStore.Create(ctx, Assignment{
			WorkItemID:       workItemID,
			AssignedByUserID: assignedByUserID,
			AssignedToUserID: assignedToUserID,
			Status:           AssignmentStatusAssigned,
			AssignedAt:       now,
		})
		if err != nil {
			return err
		}

		if err := s.recordAssignmentEvent(ctx, workItemID, AssignmentStatusAssigned, assignedByUserID, assignedToUserID, nil, now); err != nil {
			return err
		}

		return s.notify(ctx, assignedToUserID, workItemID, "assignment_created", item.ReferenceCode+" was assigned to you")
	})
	if err != nil {
		return Assignment{}, err
	}

	return assignment, nil
}

// GetAssignment returns the current assignment for a work item. It confirms
// the work item exists first, so a bad work item id and a work item that
// has never been assigned come back as two distinguishable errors instead
// of both looking like "not found" for the same reason. A non-admin caller
// only sees the assignment for a work item assigned to them.
func (s Service) GetAssignment(ctx context.Context, workItemID string, callerUserID string, callerIsAdmin bool, callerIsSupervisor bool) (Assignment, error) {
	if strings.TrimSpace(workItemID) == "" {
		return Assignment{}, ErrInvalidInput
	}

	item, err := s.store.GetByID(ctx, workItemID)
	if err != nil {
		return Assignment{}, err
	}

	allowed, err := s.mayView(ctx, item, callerUserID, callerIsAdmin, callerIsSupervisor)
	if err != nil {
		return Assignment{}, err
	}
	if !allowed {
		return Assignment{}, ErrNotFound
	}

	return s.assignmentStore.GetByWorkItemID(ctx, workItemID)
}

// RespondToAssignment lets the assignee a work item is assigned to accept
// or decline it. accept controls which path is taken:
//
//   - accept: work item moves Assigned -> Accepted, assignment recorded as
//     accepted
//   - decline: work item moves Assigned -> Created (unassigned again, so
//     an admin can reassign it), assignment recorded as declined
//
// respondingUserID must match the assignment's AssignedToUserID — this is
// the ownership check that role middleware alone cannot make, since
// middleware only knows the caller's role, not which specific work item
// they were assigned.
func (s Service) RespondToAssignment(ctx context.Context, workItemID string, respondingUserID string, accept bool, input RespondToAssignmentInput) (Assignment, error) {
	if strings.TrimSpace(workItemID) == "" || strings.TrimSpace(respondingUserID) == "" {
		return Assignment{}, ErrInvalidInput
	}

	// Same multi-store reasoning as ChangeStatus/AssignWorkItem — writes
	// to work_items, status_history, assignments, assignment_history, and
	// sometimes notifications, so it runs inside runAtomic.
	var updated Assignment

	err := s.runAtomic(ctx, func(ctx context.Context) error {
		item, err := s.store.GetByID(ctx, workItemID)
		if err != nil {
			return err
		}

		assignment, err := s.assignmentStore.GetByWorkItemID(ctx, workItemID)
		if err != nil {
			return err
		}

		if assignment.AssignedToUserID != respondingUserID {
			return ErrAssignmentNotOwned
		}

		if assignment.Status != AssignmentStatusAssigned {
			return ErrAssignmentNotPending
		}

		toStatus := StatusCreated
		assignmentStatus := AssignmentStatusDeclined
		if accept {
			toStatus = StatusAccepted
			assignmentStatus = AssignmentStatusAccepted
		}

		if !IsValidTransition(item.Status, toStatus) {
			return ErrInvalidTransition
		}

		fromStatus := item.Status
		now := s.now()

		item.Status = toStatus
		item.UpdatedAt = now

		if !accept {
			item.AssignedToUserID = nil
		}

		if _, err := s.store.Update(ctx, item); err != nil {
			return err
		}

		if err := s.recordStatusChange(ctx, workItemID, fromStatus, toStatus, respondingUserID, input.Note, now); err != nil {
			return err
		}

		var note *string
		if input.Note != nil {
			trimmed := strings.TrimSpace(*input.Note)
			if trimmed != "" {
				note = &trimmed
			}
		}

		assignment.Status = assignmentStatus
		assignment.RespondedAt = ptrTime(now)
		assignment.ResponseNote = note

		updated, err = s.assignmentStore.Update(ctx, assignment)
		if err != nil {
			return err
		}

		if err := s.recordAssignmentEvent(ctx, workItemID, assignmentStatus, respondingUserID, assignment.AssignedToUserID, input.Note, now); err != nil {
			return err
		}

		// Declining isn't in the Event Hooks list — only acceptance is.
		// The admin who made a doomed assignment finds out by checking,
		// same as today; nobody's waiting on a decline the way they're
		// waiting on an accept.
		if accept {
			return s.notify(ctx, assignment.AssignedByUserID, workItemID, "assignment_accepted", item.ReferenceCode+" was accepted by the assignee")
		}

		return nil
	})
	if err != nil {
		return Assignment{}, err
	}

	return updated, nil
}

func (s Service) WithClock(now func() time.Time) Service {
	s.now = now
	return s
}
