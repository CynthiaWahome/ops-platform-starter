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
