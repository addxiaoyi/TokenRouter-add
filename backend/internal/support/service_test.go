package support

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// service_test.go verifies boundary validation independently of persistence.
// Invalid inputs must stop before any store call, while Unicode titles and
// stable retry IDs remain valid. PostgreSQL integration tests cover ownership,
// concurrent updates, idempotency, and durable notification behavior separately.

// recordingStore captures valid writes while embedding unused store operations.
type recordingStore struct {
	Store                   // unexpected operations fail instead of manufacturing records
	created    *CreateInput // latest validated ticket submission
	status     *StatusInput // latest authorized status transition
	recipients []string     // latest normalized staff addresses
}

// Create captures validated input and supplies a deterministic committed identifier.
func (s *recordingStore) Create(_ context.Context, _ Actor, input CreateInput) (int64, error) {
	s.created = &input
	return 1, nil
}

// Get returns the committed identifier after a successful creation call.
func (s *recordingStore) Get(context.Context, Actor, int64, int64) (Thread, error) {
	return Thread{Ticket: Ticket{ID: 1}}, nil
}

// SetStatus records only state changes already authorized by service validation.
func (s *recordingStore) SetStatus(_ context.Context, _ Actor, _ int64, input StatusInput) error {
	s.status = &input
	return nil
}

// SetRecipients captures validated notification destinations without any SMTP side effect.
func (s *recordingStore) SetRecipients(_ context.Context, addresses []string) error {
	s.recipients = addresses
	return nil
}

// TestCreateRejectsInvalidInput validates titles, messages, and retry identities.
// Every rejected boundary leaves persistence untouched, and Chinese titles count
// as characters instead of consuming the limit according to UTF-8 byte length.
func TestCreateRejectsInvalidInput(t *testing.T) {
	valid := CreateInput{Subject: strings.Repeat("问", 160), Message: "测试问题", ClientID: uuid.NewString()}
	for _, mutate := range []func(*CreateInput){func(v *CreateInput) { v.Subject = "a" }, func(v *CreateInput) { v.Subject = strings.Repeat("问", 161) }, func(v *CreateInput) { v.Message = " " }, func(v *CreateInput) { v.Message = "x\x00y" }, func(v *CreateInput) { v.ClientID = "bad" }} {
		store := &recordingStore{}
		input := valid
		mutate(&input)
		_, err := NewService(store).Create(context.Background(), Actor{UserID: 1}, input)
		require.Error(t, err)
		require.Nil(t, store.created)
	}
	store := &recordingStore{}
	thread, err := NewService(store).Create(context.Background(), Actor{UserID: 1}, valid)
	require.NoError(t, err)
	require.Equal(t, int64(1), thread.Ticket.ID)
	require.Equal(t, valid, *store.created)
}

// TestStatusRequiresStaffForResolution keeps user status controls owner-safe.
// Resolution and progress require staff, while owners may explicitly close or
// reopen a known revision without gaining administrator capabilities.
func TestStatusRequiresStaffForResolution(t *testing.T) {
	store := &recordingStore{}
	service := NewService(store)
	require.Error(t, service.SetStatus(context.Background(), Actor{UserID: 1}, 1, StatusInput{Status: "resolved", Version: 1}))
	require.Nil(t, store.status)
	require.NoError(t, service.SetStatus(context.Background(), Actor{UserID: 1}, 1, StatusInput{Status: "closed", Version: 1}))
	require.NoError(t, service.SetStatus(context.Background(), Actor{UserID: 2, Staff: true}, 1, StatusInput{Status: "resolved", Version: 2}))
}

// TestRecipientsValidateBeforeReplacement preserves configuration after invalid addresses.
// Duplicate normalized addresses collapse, while an empty list remains valid
// because ticket submission never depends on configured mail recipients.
func TestRecipientsValidateBeforeReplacement(t *testing.T) {
	store := &recordingStore{}
	service := NewService(store)
	_, err := service.SetRecipients(context.Background(), []string{"valid@example.com", "bad"})
	require.Error(t, err)
	require.Nil(t, store.recipients)
	addresses, err := service.SetRecipients(context.Background(), []string{" User@example.com ", "user@example.com"})
	require.NoError(t, err)
	require.Len(t, addresses, 1)
	_, err = service.SetRecipients(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, store.recipients)
}
