package workitems

import "testing"

func TestStatusIsValid(t *testing.T) {
	valid := []Status{
		StatusCreated, StatusAssigned, StatusAccepted, StatusInProgress,
		StatusSubmittedForReview, StatusVerified, StatusFlagged,
		StatusCompleted, StatusCancelled,
	}

	for _, status := range valid {
		if !status.IsValid() {
			t.Errorf("expected %q to be valid", status)
		}
	}

	if Status("unknown").IsValid() {
		t.Error("expected unknown status to be invalid")
	}
}

func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		name string
		from Status
		to   Status
		want bool
	}{
		{"created to assigned is allowed", StatusCreated, StatusAssigned, true},
		{"created to cancelled is allowed", StatusCreated, StatusCancelled, true},
		{"created to completed is not allowed", StatusCreated, StatusCompleted, false},

		{"assigned to accepted is allowed", StatusAssigned, StatusAccepted, true},
		{"assigned to cancelled is allowed", StatusAssigned, StatusCancelled, true},

		{"accepted to in progress is allowed", StatusAccepted, StatusInProgress, true},

		{"in progress to submitted for review is allowed", StatusInProgress, StatusSubmittedForReview, true},

		{"submitted for review to verified is allowed", StatusSubmittedForReview, StatusVerified, true},
		{"submitted for review to flagged is allowed", StatusSubmittedForReview, StatusFlagged, true},

		{"verified to completed is allowed", StatusVerified, StatusCompleted, true},
		{"verified to cancelled is allowed", StatusVerified, StatusCancelled, true},

		{"flagged to in progress is allowed (rework loop)", StatusFlagged, StatusInProgress, true},

		{"completed has no outgoing transitions", StatusCompleted, StatusCancelled, false},
		{"cancelled has no outgoing transitions", StatusCancelled, StatusAssigned, false},

		{"verified and completed must stay distinct", StatusSubmittedForReview, StatusCompleted, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsValidTransition(tc.from, tc.to)
			if got != tc.want {
				t.Errorf("IsValidTransition(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

// TestIsAssigneeAllowedTransition is IsValidTransition's counterpart:
// where that table asserts what is legal at all, this one asserts what a
// non-admin caller specifically may trigger themselves. Every entry that
// is legal in the full table but absent here (assign, verify, flag,
// complete, cancel, the initial submit-for-review evidence check) is
// deliberately excluded — those stay admin-only, or in the case of
// submit-for-review, gated by a separate handler-level rule (OPS-031)
// rather than this table. OPS-042: before this test, the only coverage
// for this function was indirect, through service-level integration
// tests — this pins the full assignee-allowed surface in one place.
func TestIsAssigneeAllowedTransition(t *testing.T) {
	tests := []struct {
		name string
		from Status
		to   Status
		want bool
	}{
		{"accepted to in progress is assignee-allowed (start work)", StatusAccepted, StatusInProgress, true},
		{"in progress to submitted for review is assignee-allowed (submit)", StatusInProgress, StatusSubmittedForReview, true},
		{"flagged to in progress is assignee-allowed (rework)", StatusFlagged, StatusInProgress, true},

		{"created to assigned is not assignee-allowed (assign stays admin-only)", StatusCreated, StatusAssigned, false},
		{"submitted for review to verified is not assignee-allowed (verify stays admin-only)", StatusSubmittedForReview, StatusVerified, false},
		{"submitted for review to flagged is not assignee-allowed (flag stays admin-only)", StatusSubmittedForReview, StatusFlagged, false},
		{"verified to completed is not assignee-allowed (complete stays admin-only)", StatusVerified, StatusCompleted, false},
		{"accepted to cancelled is not assignee-allowed (cancel stays admin-only)", StatusAccepted, StatusCancelled, false},

		{"accepted to submitted for review is not assignee-allowed (skips in progress)", StatusAccepted, StatusSubmittedForReview, false},
		{"a status with no assignee-allowed entries returns false, not a panic", StatusCompleted, StatusInProgress, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsAssigneeAllowedTransition(tc.from, tc.to)
			if got != tc.want {
				t.Errorf("IsAssigneeAllowedTransition(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}
