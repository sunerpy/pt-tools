package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sunerpy/pt-tools/global"
)

func TestEncryptCookieRoundTrip(t *testing.T) {
	writeTestSecretKey(t)
	store := NewConfigStore(nil)

	for _, plain := range []string{"", "abc=1; def=2", strings.Repeat("cookie=value;", 5*1024/13+1)} {
		t.Run(plain[:min(len(plain), 12)], func(t *testing.T) {
			cipherText, err := store.EncryptCookie(plain)
			require.NoError(t, err)

			gotPlain, err := store.DecryptCookie(cipherText)
			require.NoError(t, err)
			require.Equal(t, plain, gotPlain)
		})
	}
}

func TestEncryptCookieKeyMissing(t *testing.T) {
	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	t.Setenv("HOME", t.TempDir())
	observedCore, observedLogs := observer.New(zapcore.WarnLevel)
	originalLogger := global.GlobalLogger
	global.GlobalLogger = zap.New(observedCore)
	t.Cleanup(func() { global.GlobalLogger = originalLogger })

	store := NewConfigStore(nil)
	cipherText, err := store.EncryptCookie("abc=1")
	require.Empty(t, cipherText)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrKeyMissing)
	require.Contains(t, err.Error(), "KEY")

	warnLogs := observedLogs.FilterLevelExact(zapcore.WarnLevel).All()
	require.NotEmpty(t, warnLogs)
	require.Contains(t, warnLogs[0].Message, "secret.key")
}

func TestEncryptCookieDoubleEncryptStable(t *testing.T) {
	writeTestSecretKey(t)
	store := NewConfigStore(nil)
	plain := "abc=1; def=2"

	firstCipherText, err := store.EncryptCookie(plain)
	require.NoError(t, err)
	secondCipherText, err := store.EncryptCookie(plain)
	require.NoError(t, err)
	require.NotEqual(t, firstCipherText, secondCipherText)

	firstPlain, err := store.DecryptCookie(firstCipherText)
	require.NoError(t, err)
	secondPlain, err := store.DecryptCookie(secondCipherText)
	require.NoError(t, err)
	require.Equal(t, plain, firstPlain)
	require.Equal(t, plain, secondPlain)
}

func writeTestSecretKey(t *testing.T) {
	t.Helper()
	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	keyDir := filepath.Join(homeDir, ".pt-tools")
	require.NoError(t, os.MkdirAll(keyDir, 0o700))
	keyFile := filepath.Join(keyDir, "secret.key")
	require.NoError(t, os.WriteFile(keyFile, []byte(strings.Repeat("a", 64)), 0o600))
}
