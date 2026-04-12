package platform

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/audit"
)

// --- Mocks ---

type mockUserStore struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*PlatformUser, error)
}

func (m *mockUserStore) CreateUser(ctx context.Context, email, displayName, passwordHash string, role Role, createdBy uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), nil
}
func (m *mockUserStore) GetUserByEmail(ctx context.Context, email string) (*PlatformUser, error) {
	return nil, nil
}
func (m *mockUserStore) GetUserByID(ctx context.Context, id uuid.UUID) (*PlatformUser, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return testPlatformAdmin(), nil
}
func (m *mockUserStore) ListUsers(ctx context.Context) ([]PlatformUser, error)        { return nil, nil }
func (m *mockUserStore) DeactivateUser(ctx context.Context, id uuid.UUID) error        { return nil }
func (m *mockUserStore) UpdateLastLogin(ctx context.Context, id uuid.UUID) error       { return nil }

type mockImpersonationStore struct {
	mu               sync.Mutex
	sessions         map[uuid.UUID]ActiveImpersonationSession // sessionID → session
	revoked          map[uuid.UUID]RevocationReason
	revokeErrFn      func(sessionID uuid.UUID, reason RevocationReason) error // optional per-call error
	activeSessionsErr error                                                    // returned by GetActiveSessionsForUser
}

func newMockImpersonationStore() *mockImpersonationStore {
	return &mockImpersonationStore{
		sessions: make(map[uuid.UUID]ActiveImpersonationSession),
		revoked:  make(map[uuid.UUID]RevocationReason),
	}
}

func (m *mockImpersonationStore) LogImpersonation(ctx context.Context, req ImpersonationRequest, expiresAt time.Time) (uuid.UUID, error) {
	id := uuid.New()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[id] = ActiveImpersonationSession{
		ID:             id,
		PlatformUserID: req.PlatformUserID,
		TenantID:       req.TenantID,
		TargetUserID:   req.UserID,
		Reason:         req.Reason,
		StartedAt:      time.Now().UTC(),
		ExpiresAt:      expiresAt,
	}
	return id, nil
}

func (m *mockImpersonationStore) RevokeSession(ctx context.Context, sessionID uuid.UUID, reason RevocationReason) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.revokeErrFn != nil {
		if err := m.revokeErrFn(sessionID, reason); err != nil {
			return err
		}
	}
	if _, ok := m.sessions[sessionID]; !ok {
		return ErrSessionNotFound
	}
	m.revoked[sessionID] = reason
	delete(m.sessions, sessionID)
	return nil
}

func (m *mockImpersonationStore) GetActiveSessionsForUser(ctx context.Context, targetUserID uuid.UUID) ([]ActiveImpersonationSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeSessionsErr != nil {
		return nil, m.activeSessionsErr
	}
	var out []ActiveImpersonationSession
	for _, s := range m.sessions {
		if s.TargetUserID == targetUserID {
			out = append(out, s)
		}
	}
	return out, nil
}

type mockTenantManager struct{}

func (m *mockTenantManager) ListTenants(ctx context.Context) ([]TenantSummary, error) { return nil, nil }
func (m *mockTenantManager) SuspendTenant(ctx context.Context, id uuid.UUID) error    { return nil }
func (m *mockTenantManager) ReactivateTenant(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockTenantManager) UpgradePlan(ctx context.Context, id uuid.UUID, plan string) error {
	return nil
}

type mockVerticalManager struct{}

func (m *mockVerticalManager) ListVerticals(ctx context.Context) ([]VerticalSummary, error) {
	return nil, nil
}
func (m *mockVerticalManager) DeactivateVertical(ctx context.Context, id string) error { return nil }

type mockLogger struct{}

func (mockLogger) Info(msg string, kv ...interface{})  {}
func (mockLogger) Warn(msg string, kv ...interface{})  {}
func (mockLogger) Error(msg string, kv ...interface{}) {}

// mockAuditSink captures LogAction calls for assertions.
type mockAuditSink struct {
	mu     sync.Mutex
	events []auditEvent
}

type auditEvent struct {
	action       audit.Action
	resourceType audit.ResourceType
	resourceID   uuid.UUID
	metadata     map[string]interface{}
}

func (m *mockAuditSink) LogAction(
	_ uuid.UUID, _ uuid.UUID, _ string,
	action audit.Action,
	resourceType audit.ResourceType,
	resourceID uuid.UUID,
	_, _ interface{},
	metadata map[string]interface{},
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, auditEvent{
		action:       action,
		resourceType: resourceType,
		resourceID:   resourceID,
		metadata:     metadata,
	})
}

func (m *mockAuditSink) actions() []audit.Action {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]audit.Action, len(m.events))
	for i, e := range m.events {
		out[i] = e.action
	}
	return out
}

// --- Helpers ---

var (
	testPlatformUserID = uuid.MustParse("f0000000-0000-0000-0000-000000000001")
	testTenantID       = uuid.MustParse("d0000000-0000-0000-0000-000000000001")
	testImpersonateID  = uuid.MustParse("d1000000-0000-0000-0000-000000000003")
)

func testPlatformAdmin() *PlatformUser {
	return &PlatformUser{
		ID:          testPlatformUserID,
		Email:       "admin@wegofwd2020.com",
		DisplayName: "Platform Admin",
		Role:        RoleAdmin,
		MFAEnabled:  true,
		Status:      "active",
	}
}

func newTestService(opts ...func(*Service)) *Service {
	s := NewService(
		&mockUserStore{},
		&mockTenantManager{},
		newMockImpersonationStore(),
		&mockVerticalManager{},
		mockLogger{},
	)
	for _, fn := range opts {
		fn(s)
	}
	return s
}

// newTestServiceWithStore returns a Service wired to a specific mock store
// so tests can inspect what was logged/revoked.
func newTestServiceWithStore(store *mockImpersonationStore, sink *mockAuditSink) *Service {
	s := NewService(
		&mockUserStore{},
		&mockTenantManager{},
		store,
		&mockVerticalManager{},
		mockLogger{},
	)
	if sink != nil {
		s.WithAuditSink(sink)
	}
	return s
}

// --- Tests ---

func TestImpersonate_HappyPath(t *testing.T) {
	t.Parallel()
	s := newTestService()

	session, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "Support ticket #1234",
		IPAddress:      "10.0.0.1",
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, session.ID)
	assert.False(t, session.ExpiresAt.IsZero())
}

func TestImpersonate_ReasonRequired(t *testing.T) {
	t.Parallel()
	s := newTestService()

	_, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReasonRequired)
}

func TestImpersonate_SupportRoleDenied(t *testing.T) {
	t.Parallel()
	s := newTestService(func(s *Service) {
		s.users = &mockUserStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*PlatformUser, error) {
				u := testPlatformAdmin()
				u.Role = RoleSupport
				return u, nil
			},
		}
	})

	_, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "Support ticket #1234",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrImpersonationDenied)
}

func TestImpersonate_MFARequired(t *testing.T) {
	t.Parallel()
	s := newTestService(func(s *Service) {
		s.users = &mockUserStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*PlatformUser, error) {
				u := testPlatformAdmin()
				u.MFAEnabled = false
				return u, nil
			},
		}
	})

	_, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "Support ticket #1234",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMFARequired)
}

func TestImpersonate_UserNotFound(t *testing.T) {
	t.Parallel()
	s := newTestService(func(s *Service) {
		s.users = &mockUserStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*PlatformUser, error) {
				return nil, errors.New("not found")
			},
		}
	})

	_, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "Support ticket #1234",
	})
	require.Error(t, err)
}

func TestCheckAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		role     Role
		required Role
		wantErr  bool
	}{
		{"owner can do anything", RoleOwner, RoleOwner, false},
		{"owner can do admin tasks", RoleOwner, RoleAdmin, false},
		{"admin can do admin tasks", RoleAdmin, RoleAdmin, false},
		{"admin can do support tasks", RoleAdmin, RoleSupport, false},
		{"support can do support tasks", RoleSupport, RoleSupport, false},
		{"support cannot do admin tasks", RoleSupport, RoleAdmin, true},
		{"admin cannot do owner tasks", RoleAdmin, RoleOwner, true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			user := &PlatformUser{Role: tc.role}
			err := CheckAccess(user, tc.required)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSeedPlatformOwner(t *testing.T) {
	t.Parallel()
	s := newTestService()

	id, err := s.SeedPlatformOwner(context.Background(), "cto@wegofwd2020.com", "CTO", "$2a$12$hash")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)
}

// --- Session duration ---

func TestImpersonate_SessionDuration(t *testing.T) {
	t.Parallel()
	store := newMockImpersonationStore()
	s := newTestServiceWithStore(store, nil)

	before := time.Now().UTC()
	session, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "Support ticket #99",
	})
	require.NoError(t, err)

	// ExpiresAt must be within MaxImpersonationDuration from now.
	assert.WithinDuration(t, before.Add(MaxImpersonationDuration), session.ExpiresAt, 2*time.Second)
	// StartedAt must be recent.
	assert.WithinDuration(t, before, session.StartedAt, 2*time.Second)
}

// --- EndImpersonation ---

func TestEndImpersonation_HappyPath(t *testing.T) {
	t.Parallel()
	store := newMockImpersonationStore()
	sink := &mockAuditSink{}
	s := newTestServiceWithStore(store, sink)

	session, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "Support ticket #99",
	})
	require.NoError(t, err)

	err = s.EndImpersonation(context.Background(), testPlatformUserID, session.ID)
	require.NoError(t, err)

	// Session must be revoked with reason=manual.
	reason, ok := store.revoked[session.ID]
	require.True(t, ok, "session should be in revoked map")
	assert.Equal(t, RevocationManual, reason)
}

func TestEndImpersonation_SessionNotFound(t *testing.T) {
	t.Parallel()
	s := newTestService()

	err := s.EndImpersonation(context.Background(), testPlatformUserID, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

// --- Revocation triggers ---

func TestRevokeOnPasswordChange_RevokesActiveSessions(t *testing.T) {
	t.Parallel()
	store := newMockImpersonationStore()
	sink := &mockAuditSink{}
	s := newTestServiceWithStore(store, sink)

	// Start two sessions targeting the same user.
	sess1, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID, TenantID: testTenantID,
		UserID: testImpersonateID, Reason: "Ticket A",
	})
	require.NoError(t, err)
	sess2, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID, TenantID: testTenantID,
		UserID: testImpersonateID, Reason: "Ticket B",
	})
	require.NoError(t, err)

	err = s.RevokeOnPasswordChange(context.Background(), testImpersonateID)
	require.NoError(t, err)

	// Both sessions should be revoked with the correct reason.
	assert.Equal(t, RevocationPasswordChange, store.revoked[sess1.ID])
	assert.Equal(t, RevocationPasswordChange, store.revoked[sess2.ID])
}

func TestRevokeOnDeactivation_RevokesActiveSessions(t *testing.T) {
	t.Parallel()
	store := newMockImpersonationStore()
	s := newTestServiceWithStore(store, nil)

	session, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID, TenantID: testTenantID,
		UserID: testImpersonateID, Reason: "Ticket C",
	})
	require.NoError(t, err)

	require.NoError(t, s.RevokeOnDeactivation(context.Background(), testImpersonateID))
	assert.Equal(t, RevocationDeactivated, store.revoked[session.ID])
}

func TestRevokeOnMFAChange_RevokesActiveSessions(t *testing.T) {
	t.Parallel()
	store := newMockImpersonationStore()
	s := newTestServiceWithStore(store, nil)

	session, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID, TenantID: testTenantID,
		UserID: testImpersonateID, Reason: "Ticket D",
	})
	require.NoError(t, err)

	require.NoError(t, s.RevokeOnMFAChange(context.Background(), testImpersonateID))
	assert.Equal(t, RevocationMFAChange, store.revoked[session.ID])
}

func TestRevokeOnPasswordChange_NoActiveSessions(t *testing.T) {
	t.Parallel()
	s := newTestService()
	// No sessions for this user — should be a no-op.
	err := s.RevokeOnPasswordChange(context.Background(), uuid.New())
	assert.NoError(t, err)
}

// --- Audit trail ---

func TestImpersonate_EmitsAuditEvent(t *testing.T) {
	t.Parallel()
	store := newMockImpersonationStore()
	sink := &mockAuditSink{}
	s := newTestServiceWithStore(store, sink)

	_, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID, TenantID: testTenantID,
		UserID: testImpersonateID, Reason: "Support ticket #100",
	})
	require.NoError(t, err)

	actions := sink.actions()
	require.Len(t, actions, 1)
	assert.Equal(t, audit.ActionImpersonationStarted, actions[0])
}

func TestEndImpersonation_EmitsAuditEvent(t *testing.T) {
	t.Parallel()
	store := newMockImpersonationStore()
	sink := &mockAuditSink{}
	s := newTestServiceWithStore(store, sink)

	session, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID, TenantID: testTenantID,
		UserID: testImpersonateID, Reason: "Support ticket #101",
	})
	require.NoError(t, err)

	require.NoError(t, s.EndImpersonation(context.Background(), testPlatformUserID, session.ID))

	actions := sink.actions()
	require.Len(t, actions, 2)
	assert.Equal(t, audit.ActionImpersonationStarted, actions[0])
	assert.Equal(t, audit.ActionImpersonationEnded, actions[1])
}

// --- Blocked actions ---

// ─── WithNotifier ─────────────────────────────────────────────────────────────

// captureNotifier records calls to NotifyImpersonationEnded.
type captureNotifier struct {
	mu    sync.Mutex
	calls []struct {
		sess   ActiveImpersonationSession
		reason RevocationReason
	}
	notify chan struct{}
}

func newCaptureNotifier() *captureNotifier {
	return &captureNotifier{notify: make(chan struct{}, 10)}
}

func (n *captureNotifier) NotifyImpersonationEnded(_ context.Context, sess ActiveImpersonationSession, reason RevocationReason) error {
	n.mu.Lock()
	n.calls = append(n.calls, struct {
		sess   ActiveImpersonationSession
		reason RevocationReason
	}{sess, reason})
	n.mu.Unlock()
	n.notify <- struct{}{}
	return nil
}

func (n *captureNotifier) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.calls)
}

func TestWithNotifier_IsCalledOnPasswordChangeRevocation(t *testing.T) {
	t.Parallel()
	store := newMockImpersonationStore()
	notifier := newCaptureNotifier()

	s := newTestServiceWithStore(store, nil)
	s.WithNotifier(notifier)

	// Create an active session targeting testImpersonateID.
	_, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "Notifier test",
	})
	require.NoError(t, err)

	// Trigger revocation — notifier is called in a background goroutine.
	require.NoError(t, s.RevokeOnPasswordChange(context.Background(), testImpersonateID))

	// Wait for the goroutine to deliver the notification (or fail after 1 s).
	select {
	case <-notifier.notify:
	case <-time.After(time.Second):
		t.Fatal("notifier was not called within 1 s")
	}

	assert.Equal(t, 1, notifier.count(), "notifier must be called exactly once")
}

// ─── revokeAllForUser error paths ────────────────────────────────────────────

func TestRevokeAllForUser_GetSessionsError_ReturnsError(t *testing.T) {
	t.Parallel()
	store := newMockImpersonationStore()
	store.activeSessionsErr = errors.New("db connection refused")
	s := newTestServiceWithStore(store, nil)

	err := s.RevokeOnPasswordChange(context.Background(), uuid.New())
	assert.Error(t, err)
}

func TestRevokeAllForUser_PartialRevokeFailure_ContinuesBestEffort(t *testing.T) {
	t.Parallel()
	store := newMockImpersonationStore()
	s := newTestServiceWithStore(store, nil)
	ctx := context.Background()

	// Two sessions target the same user.
	sess1, err := s.Impersonate(ctx, ImpersonationRequest{
		PlatformUserID: testPlatformUserID, TenantID: testTenantID,
		UserID: testImpersonateID, Reason: "Ticket E",
	})
	require.NoError(t, err)
	sess2, err := s.Impersonate(ctx, ImpersonationRequest{
		PlatformUserID: testPlatformUserID, TenantID: testTenantID,
		UserID: testImpersonateID, Reason: "Ticket F",
	})
	require.NoError(t, err)

	// Make RevokeSession fail specifically for sess1.
	store.revokeErrFn = func(sessionID uuid.UUID, _ RevocationReason) error {
		if sessionID == sess1.ID {
			return errors.New("transient lock timeout")
		}
		return nil
	}

	// Overall call must still succeed (best-effort — partial failures are logged).
	require.NoError(t, s.RevokeOnPasswordChange(ctx, testImpersonateID))

	// sess1 failed revocation: must not appear in revoked map.
	_, wasRevoked := store.revoked[sess1.ID]
	assert.False(t, wasRevoked, "failed session must not be marked revoked")

	// sess2 should be revoked successfully.
	assert.Equal(t, RevocationPasswordChange, store.revoked[sess2.ID])
}

// ─── CheckAccess unknown role ─────────────────────────────────────────────────

func TestCheckAccess_UnknownUserRole_ReturnsNotPlatformUser(t *testing.T) {
	t.Parallel()
	user := &PlatformUser{Role: Role("unknown-role")}
	err := CheckAccess(user, RoleAdmin)
	assert.ErrorIs(t, err, ErrNotPlatformUser)
}

// ─── SeedPlatformOwner error path ─────────────────────────────────────────────

func TestSeedPlatformOwner_StoreError_ReturnsWrappedError(t *testing.T) {
	t.Parallel()
	s := newTestService(func(s *Service) {
		s.users = &mockUserStore{
			getByIDFn: func(_ context.Context, _ uuid.UUID) (*PlatformUser, error) {
				return testPlatformAdmin(), nil
			},
		}
		// Override CreateUser to return an error.
		// mockUserStore.CreateUser always returns uuid.New(), nil so we need a custom store.
	})

	// Replace the user store with one that fails CreateUser.
	failingStore := &failingCreateUserStore{}
	s.users = failingStore

	_, err := s.SeedPlatformOwner(context.Background(), "owner@example.com", "Owner", "$argon2id$hash")
	assert.Error(t, err)
}

// failingCreateUserStore returns an error from CreateUser.
type failingCreateUserStore struct{ mockUserStore }

func (f *failingCreateUserStore) CreateUser(_ context.Context, _, _, _ string, _ Role, _ uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, errors.New("platform: db write failed")
}

func TestIsActionBlocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action  string
		blocked bool
	}{
		{"ChangePassword", true},
		{"UpdateMFA", true},
		{"DisableMFA", true},
		{"EnableMFA", true},
		{"RevokeAllTokens", true},
		{"CreateSubscription", true},
		{"UpgradeSubscription", true},
		{"CancelSubscription", true},
		{"AddPaymentMethod", true},
		{"RemovePaymentMethod", true},
		{"AssignRole", true},
		{"RevokeRole", true},
		// These must not be blocked.
		{"GetUser", false},
		{"ListExpenses", false},
		{"CreateProduction", false},
		{"ApproveExpense", false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.action, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.blocked, IsActionBlocked(tc.action))
		})
	}
}
