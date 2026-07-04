package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mkandel/go-checklists/internal/auth"
	"github.com/mkandel/go-checklists/internal/domain"
)

type fakeUserRepo struct {
	byUsername map[string]*domain.User
	byID       map[int64]*domain.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byUsername: map[string]*domain.User{}, byID: map[int64]*domain.User{}}
}

func (f *fakeUserRepo) add(u *domain.User) {
	f.byUsername[u.Username] = u
	f.byID[u.ID] = u
}

func (f *fakeUserRepo) Create(ctx context.Context, u *domain.User) error { f.add(u); return nil }

func (f *fakeUserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, errors.New("fake: user not found")
	}
	return u, nil
}

func (f *fakeUserRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	u, ok := f.byUsername[username]
	if !ok {
		return nil, errors.New("fake: user not found")
	}
	return u, nil
}

func (f *fakeUserRepo) List(ctx context.Context) ([]domain.User, error) {
	var out []domain.User
	for _, u := range f.byID {
		out = append(out, *u)
	}
	return out, nil
}

type fakeSessionRepo struct {
	byToken map[string]*domain.Session
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{byToken: map[string]*domain.Session{}}
}

func (f *fakeSessionRepo) Create(ctx context.Context, s *domain.Session) error {
	f.byToken[s.Token] = s
	return nil
}

func (f *fakeSessionRepo) Get(ctx context.Context, token string) (*domain.Session, error) {
	s, ok := f.byToken[token]
	if !ok {
		return nil, errors.New("fake: session not found")
	}
	return s, nil
}

func (f *fakeSessionRepo) Delete(ctx context.Context, token string) error {
	delete(f.byToken, token)
	return nil
}

func (f *fakeSessionRepo) Refresh(ctx context.Context, token string, newExpiresAt time.Time) error {
	s, ok := f.byToken[token]
	if !ok {
		return errors.New("fake: session not found")
	}
	s.ExpiresAt = newExpiresAt
	return nil
}

func (f *fakeSessionRepo) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	var n int64
	for token, s := range f.byToken {
		if s.ExpiresAt.Before(now) {
			delete(f.byToken, token)
			n++
		}
	}
	return n, nil
}

func mustHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return hash
}

func TestHashPassword_RoundTrip(t *testing.T) {
	hash := mustHash(t, "correct horse battery staple")
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("correct horse battery staple")); err != nil {
		t.Fatalf("expected hash to verify, got: %v", err)
	}
}

func TestLogin_Success(t *testing.T) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()
	users.add(&domain.User{ID: 1, Username: "alice", PasswordHash: mustHash(t, "hunter2"), IsActive: true})

	before := time.Now()
	s, err := auth.Login(context.Background(), users, sessions, "alice", "hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if s.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if s.UserID != 1 {
		t.Fatalf("UserID = %d, want 1", s.UserID)
	}
	wantExpiry := before.Add(auth.SessionTTL)
	if s.ExpiresAt.Before(wantExpiry.Add(-time.Second)) || s.ExpiresAt.After(wantExpiry.Add(time.Second)) {
		t.Fatalf("ExpiresAt = %v, want ~%v", s.ExpiresAt, wantExpiry)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()
	users.add(&domain.User{ID: 1, Username: "alice", PasswordHash: mustHash(t, "hunter2"), IsActive: true})

	_, err := auth.Login(context.Background(), users, sessions, "alice", "wrong")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()

	_, err := auth.Login(context.Background(), users, sessions, "nobody", "whatever")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLogin_InactiveUser(t *testing.T) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()
	users.add(&domain.User{ID: 1, Username: "alice", PasswordHash: mustHash(t, "hunter2"), IsActive: false})

	_, err := auth.Login(context.Background(), users, sessions, "alice", "hunter2")
	if !errors.Is(err, auth.ErrInactiveUser) {
		t.Fatalf("err = %v, want ErrInactiveUser", err)
	}
}

func TestLogout_DeletesSession(t *testing.T) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()
	users.add(&domain.User{ID: 1, Username: "alice", PasswordHash: mustHash(t, "hunter2"), IsActive: true})

	s, err := auth.Login(context.Background(), users, sessions, "alice", "hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := auth.Logout(context.Background(), sessions, s.Token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, _, _, err := auth.CurrentUser(context.Background(), users, sessions, s.Token); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("CurrentUser after logout: err = %v, want ErrSessionNotFound", err)
	}
}

func TestLogout_UnknownToken_NoError(t *testing.T) {
	sessions := newFakeSessionRepo()
	if err := auth.Logout(context.Background(), sessions, "nonexistent"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
}

func TestCurrentUser_Success(t *testing.T) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()
	users.add(&domain.User{ID: 1, Username: "alice", PasswordHash: mustHash(t, "hunter2"), IsActive: true})

	s, err := auth.Login(context.Background(), users, sessions, "alice", "hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	u, _, renewed, err := auth.CurrentUser(context.Background(), users, sessions, s.Token)
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if u.Username != "alice" {
		t.Fatalf("Username = %q, want alice", u.Username)
	}
	if renewed {
		t.Fatal("expected a freshly-created session not to be renewed")
	}
}

func TestCurrentUser_Expired(t *testing.T) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()
	users.add(&domain.User{ID: 1, Username: "alice", IsActive: true})
	sessions.Create(context.Background(), &domain.Session{
		Token: "expired-token", UserID: 1,
		CreatedAt: time.Now().Add(-2 * auth.SessionTTL),
		ExpiresAt: time.Now().Add(-time.Minute),
	})

	_, _, _, err := auth.CurrentUser(context.Background(), users, sessions, "expired-token")
	if !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestCurrentUser_UnknownToken(t *testing.T) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()

	_, _, _, err := auth.CurrentUser(context.Background(), users, sessions, "nonexistent")
	if !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestCurrentUser_RenewsNearExpiry(t *testing.T) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()
	users.add(&domain.User{ID: 1, Username: "alice", IsActive: true})
	sessions.Create(context.Background(), &domain.Session{
		Token:     "near-expiry-token",
		UserID:    1,
		CreatedAt: time.Now().Add(-auth.SessionTTL),
		ExpiresAt: time.Now().Add(time.Minute),
	})

	_, s, renewed, err := auth.CurrentUser(context.Background(), users, sessions, "near-expiry-token")
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if !renewed {
		t.Fatal("expected renewed = true for a near-expiry session")
	}
	wantExpiry := time.Now().Add(auth.SessionTTL)
	if s.ExpiresAt.Before(wantExpiry.Add(-time.Second)) || s.ExpiresAt.After(wantExpiry.Add(time.Second)) {
		t.Fatalf("ExpiresAt = %v, want ~%v", s.ExpiresAt, wantExpiry)
	}
	stored, err := sessions.Get(context.Background(), "near-expiry-token")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !stored.ExpiresAt.Equal(s.ExpiresAt) {
		t.Fatalf("stored ExpiresAt = %v, want %v", stored.ExpiresAt, s.ExpiresAt)
	}
}

func TestCurrentUser_NoRenewalFarFromExpiry(t *testing.T) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()
	users.add(&domain.User{ID: 1, Username: "alice", IsActive: true})
	originalExpiry := time.Now().Add(auth.SessionTTL)
	sessions.Create(context.Background(), &domain.Session{
		Token:     "fresh-token",
		UserID:    1,
		CreatedAt: time.Now(),
		ExpiresAt: originalExpiry,
	})

	_, s, renewed, err := auth.CurrentUser(context.Background(), users, sessions, "fresh-token")
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if renewed {
		t.Fatal("expected renewed = false for a freshly-created session")
	}
	if !s.ExpiresAt.Equal(originalExpiry) {
		t.Fatalf("ExpiresAt = %v, want unchanged %v", s.ExpiresAt, originalExpiry)
	}
}

func TestLoginLimiter_BlocksAfterThreshold(t *testing.T) {
	l := auth.NewLoginLimiter(3, time.Minute)
	key := "1.2.3.4"

	for i := 0; i < 3; i++ {
		if !l.Allow(key) {
			t.Fatalf("expected Allow to be true before threshold, attempt %d", i)
		}
		l.RecordFailure(key)
	}
	if l.Allow(key) {
		t.Fatal("expected Allow to be false after threshold reached")
	}
}

func TestLoginLimiter_SuccessClearsWindow(t *testing.T) {
	l := auth.NewLoginLimiter(2, time.Minute)
	key := "1.2.3.4"

	l.RecordFailure(key)
	l.RecordFailure(key)
	if l.Allow(key) {
		t.Fatal("expected Allow to be false after threshold reached")
	}
	l.RecordSuccess(key)
	if !l.Allow(key) {
		t.Fatal("expected Allow to be true after a successful login clears the window")
	}
}

func TestLoginLimiter_DistinctKeysIndependent(t *testing.T) {
	l := auth.NewLoginLimiter(1, time.Minute)
	l.RecordFailure("1.2.3.4")
	if l.Allow("1.2.3.4") {
		t.Fatal("expected key 1.2.3.4 to be blocked")
	}
	if !l.Allow("5.6.7.8") {
		t.Fatal("expected a distinct key to be unaffected")
	}
}
