package identity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsReservedEmail_DingTalkDomain supplies lowercase and uppercase DingTalk
// synthetic addresses and a real-domain address. It requires reserved-domain
// recognition across case while leaving the real address unreserved.
func TestIsReservedEmail_DingTalkDomain(t *testing.T) {
	require.True(t, IsReservedEmail("dingtalk-123@dingtalk-connect.invalid"))
	require.True(t, IsReservedEmail("DINGTALK-456@DINGTALK-CONNECT.INVALID")) // case-insensitive
	require.False(t, IsReservedEmail("real@dingtalk.com"))
}

// TestIsReservedEmail_LegacyMigration distinguishes a mailbox-less imported
// account from a real site mailbox. Binding must allow its owner to verify a
// real address and establish a password for the first time.
func TestIsReservedEmail_LegacyMigration(t *testing.T) {
	email := "legacy-123" + LegacyMigrationSyntheticEmailDomain
	require.True(t, IsReservedEmail(email))
	require.True(t, IsReservedEmail(" LEGACY-123@LEGACY-MIGRATION.INVALID "))
	require.False(t, AuthHasBindableEmailIdentitySubject(email))
	require.Empty(t, AuthNormalizeBoundEmailAuthIdentitySubject(email))
	require.False(t, IsReservedEmail("real@star-mc.top"))
}
