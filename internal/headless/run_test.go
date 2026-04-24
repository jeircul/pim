package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jeircul/pim/internal/app"
	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/state"
)

type mockClient struct {
	user          *azure.User
	userErr       error
	active        []azure.ActiveAssignment
	activeErr     error
	eligible      []azure.Role
	eligibleErr   error
	activateErr   error
	deactivateErr error
}

func (m *mockClient) GetCurrentUser(_ context.Context) (*azure.User, error) {
	return m.user, m.userErr
}

func (m *mockClient) GetActiveAssignments(_ context.Context) ([]azure.ActiveAssignment, error) {
	return m.active, m.activeErr
}

func (m *mockClient) GetEligibleRoles(_ context.Context) ([]azure.Role, error) {
	return m.eligible, m.eligibleErr
}

func (m *mockClient) ActivateRole(_ context.Context, role azure.Role, principalID, justification string, minutes int, targetScope string) (*azure.ScheduleResponse, error) {
	if m.activateErr != nil {
		return nil, m.activateErr
	}
	return &azure.ScheduleResponse{}, nil
}

func (m *mockClient) DeactivateRole(_ context.Context, assignment azure.ActiveAssignment, principalID string) (*azure.ScheduleResponse, error) {
	if m.deactivateErr != nil {
		return nil, m.deactivateErr
	}
	return &azure.ScheduleResponse{}, nil
}

func newTestApp(t *testing.T, cfg app.Config) *app.App {
	t.Helper()
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}
	return &app.App{Store: store, Config: cfg}
}

func captureOutput(t *testing.T, fn func(w io.Writer) error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	fnErr := fn(w)
	w.Close()
	out, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(out), fnErr
}

func TestRunActivate(t *testing.T) {
	user := &azure.User{ID: "uid-1", UserPrincipalName: "user@example.com"}
	eligibleRole := azure.Role{
		RoleName:              "Contributor",
		Scope:                 "/subscriptions/sub-1",
		RoleDefinitionID:      "rd-1",
		EligibilityScheduleID: "/sched/1",
	}

	tests := []struct {
		name    string
		cfg     app.Config
		client  *mockClient
		wantErr string
		wantOut string
	}{
		{
			name: "missing required flags no role",
			cfg: app.Config{
				Command:       app.CmdActivate,
				Scopes:        []string{"/subscriptions/sub-1"},
				TimeStr:       "1h",
				Justification: "need access",
			},
			client:  &mockClient{user: user},
			wantErr: "requires",
		},
		{
			name: "no matching eligible roles",
			cfg: app.Config{
				Command:       app.CmdActivate,
				Roles:         []string{"Owner"},
				Scopes:        []string{"/subscriptions/sub-1"},
				TimeStr:       "1h",
				Justification: "need access",
			},
			client: &mockClient{
				user:     user,
				eligible: []azure.Role{eligibleRole},
			},
			wantErr: "no eligible roles",
		},
		{
			name: "successful activation",
			cfg: app.Config{
				Command:       app.CmdActivate,
				Roles:         []string{"Contributor"},
				Scopes:        []string{"/subscriptions/sub-1"},
				TimeStr:       "1h",
				Justification: "need access",
			},
			client: &mockClient{
				user:     user,
				eligible: []azure.Role{eligibleRole},
			},
			wantOut: "Activated:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp(t, tc.cfg)
			out, err := captureOutput(t, func(w io.Writer) error {
				return runActivate(context.Background(), a, tc.client, user, w)
			})
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantOut != "" && !strings.Contains(out, tc.wantOut) {
				t.Errorf("output %q does not contain %q", out, tc.wantOut)
			}
		})
	}
}

func TestRunDeactivate(t *testing.T) {
	user := &azure.User{ID: "uid-1"}
	activeAssignment := azure.ActiveAssignment{
		RoleName:         "Contributor",
		Scope:            "/subscriptions/sub-1",
		ScopeDisplay:     "My Sub",
		RoleDefinitionID: "rd-1",
	}

	tests := []struct {
		name    string
		cfg     app.Config
		client  *mockClient
		wantErr string
		wantOut string
	}{
		{
			name: "no filters no yes flag",
			cfg: app.Config{
				Command: app.CmdDeactivate,
			},
			client:  &mockClient{user: user, active: []azure.ActiveAssignment{activeAssignment}},
			wantErr: "--headless deactivate requires",
		},
		{
			name: "no matching active assignments with filter returns error",
			cfg: app.Config{
				Command: app.CmdDeactivate,
				Roles:   []string{"Owner"},
			},
			client: &mockClient{
				user:   user,
				active: []azure.ActiveAssignment{activeAssignment},
			},
			wantErr: "no active assignments match",
		},
		{
			name: "successful deactivation",
			cfg: app.Config{
				Command: app.CmdDeactivate,
				Roles:   []string{"Contributor"},
			},
			client: &mockClient{
				user:   user,
				active: []azure.ActiveAssignment{activeAssignment},
			},
			wantOut: "Deactivated:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp(t, tc.cfg)
			out, err := captureOutput(t, func(w io.Writer) error {
				return runDeactivate(context.Background(), a, tc.client, user, w)
			})
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantOut != "" && !strings.Contains(out, tc.wantOut) {
				t.Errorf("output %q does not contain %q", out, tc.wantOut)
			}
		})
	}
}

func TestRunDeactivateYesFlag(t *testing.T) {
	user := &azure.User{ID: "uid-1"}
	activeAssignment := azure.ActiveAssignment{
		RoleName:         "Contributor",
		Scope:            "/subscriptions/sub-1",
		ScopeDisplay:     "My Sub",
		RoleDefinitionID: "rd-1",
	}

	a := newTestApp(t, app.Config{
		Command: app.CmdDeactivate,
		Yes:     true,
	})
	client := &mockClient{
		user:   user,
		active: []azure.ActiveAssignment{activeAssignment},
	}

	out, err := captureOutput(t, func(w io.Writer) error {
		return runDeactivate(context.Background(), a, client, user, w)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Deactivated:") {
		t.Errorf("output %q does not contain %q", out, "Deactivated:")
	}
}

func TestRunDeactivateClientError(t *testing.T) {
	user := &azure.User{ID: "uid-1"}
	a := newTestApp(t, app.Config{
		Command: app.CmdDeactivate,
		Roles:   []string{"Contributor"},
	})
	client := &mockClient{
		user:      user,
		activeErr: fmt.Errorf("network error"),
	}

	_, err := captureOutput(t, func(w io.Writer) error {
		return runDeactivate(context.Background(), a, client, user, w)
	})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "get active assignments") {
		t.Errorf("error %q does not contain %q", err.Error(), "get active assignments")
	}
}

func TestRunStatus(t *testing.T) {
	user := &azure.User{ID: "uid-1"}
	activeAssignment := azure.ActiveAssignment{
		RoleName:     "Contributor",
		Scope:        "/subscriptions/sub-1",
		ScopeDisplay: "My Sub",
		EndDateTime:  "",
	}

	tests := []struct {
		name    string
		cfg     app.Config
		client  *mockClient
		wantOut string
	}{
		{
			name: "no active assignments",
			cfg:  app.Config{Command: app.CmdStatus},
			client: &mockClient{
				user:   user,
				active: nil,
			},
			wantOut: "No active PIM",
		},
		{
			name: "json output",
			cfg: app.Config{
				Command: app.CmdStatus,
				Output:  app.OutputJSON,
			},
			client: &mockClient{
				user:   user,
				active: []azure.ActiveAssignment{activeAssignment},
			},
			wantOut: "[",
		},
		{
			name: "table output contains ROLE header",
			cfg: app.Config{
				Command: app.CmdStatus,
				Output:  app.OutputTable,
			},
			client: &mockClient{
				user:   user,
				active: []azure.ActiveAssignment{activeAssignment},
			},
			wantOut: "ROLE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp(t, tc.cfg)
			out, err := captureOutput(t, func(w io.Writer) error {
				return runStatus(context.Background(), a, tc.client, user, w)
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantOut != "" && !strings.Contains(out, tc.wantOut) {
				t.Errorf("output %q does not contain %q", out, tc.wantOut)
			}
		})
	}
}

func TestRunStatusJSONValid(t *testing.T) {
	user := &azure.User{ID: "uid-1"}
	a := newTestApp(t, app.Config{Command: app.CmdStatus, Output: app.OutputJSON})
	client := &mockClient{
		user: user,
		active: []azure.ActiveAssignment{
			{RoleName: "Owner", Scope: "/subscriptions/sub-1", ScopeDisplay: "Sub 1"},
		},
	}

	out, err := captureOutput(t, func(w io.Writer) error {
		return runStatus(context.Background(), a, client, user, w)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []azure.ActiveAssignment
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Errorf("output is not valid JSON array: %v\noutput: %s", err, out)
	}
	if len(result) != 1 {
		t.Errorf("want 1 element, got %d", len(result))
	}
}
